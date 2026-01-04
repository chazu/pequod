package reconcile

import (
	"context"
	"fmt"

	"cuelang.org/go/cue"
	"github.com/authzed/controller-idioms/pause"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	platformv1alpha1 "github.com/chazu/pequod/api/v1alpha1"
	"github.com/chazu/pequod/pkg/crd"
	"github.com/chazu/pequod/pkg/platformloader"
	"github.com/chazu/pequod/pkg/schema"
)

const (
	// PausedAnnotation is the annotation key to pause reconciliation
	PausedAnnotation = "platform.pequod.io/paused"

	// TransformFinalizer is the finalizer added to Transform resources
	TransformFinalizer = "platform.pequod.io/transform-finalizer"
)

// TransformHandlers contains all handlers for Transform reconciliation
type TransformHandlers struct {
	client    client.Client
	scheme    *runtime.Scheme
	recorder  record.EventRecorder
	loader    *platformloader.Loader
	extractor *schema.Extractor
	generator *crd.Generator
}

// NewTransformHandlers creates a new handler collection for Transform
func NewTransformHandlers(
	k8sClient client.Client,
	scheme *runtime.Scheme,
	recorder record.EventRecorder,
	loader *platformloader.Loader,
) *TransformHandlers {
	return &TransformHandlers{
		client:    k8sClient,
		scheme:    scheme,
		recorder:  recorder,
		loader:    loader,
		extractor: schema.NewExtractor(),
		generator: crd.NewGenerator(),
	}
}

// Reconcile executes the full reconciliation pipeline for a Transform.
// In the new architecture, Transform generates a CRD from the CUE schema.
func (h *TransformHandlers) Reconcile(ctx context.Context, nn types.NamespacedName) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Step 1: Fetch the Transform
	tf := &platformv1alpha1.Transform{}
	if err := h.client.Get(ctx, nn, tf); err != nil {
		if errors.IsNotFound(err) {
			// Resource deleted - stop reconciliation
			logger.Info("Transform not found, ignoring")
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Step 2: Handle finalizer
	if !tf.DeletionTimestamp.IsZero() {
		// Transform is being deleted
		return h.handleDeletion(ctx, tf)
	}

	// Ensure finalizer is present
	if !controllerutil.ContainsFinalizer(tf, TransformFinalizer) {
		logger.Info("Adding finalizer to Transform")
		controllerutil.AddFinalizer(tf, TransformFinalizer)
		if err := h.client.Update(ctx, tf); err != nil {
			logger.Error(err, "Failed to add finalizer")
			return ctrl.Result{}, err
		}
		// Requeue to continue reconciliation with finalizer in place
		return ctrl.Result{Requeue: true}, nil
	}

	// Step 3: Check if paused
	if pause.IsPaused(tf, PausedAnnotation) {
		logger.Info("Transform is paused, skipping reconciliation", "annotation", PausedAnnotation)

		// Only update Paused condition if not already set to True
		existingCond := tf.GetCondition(pause.ConditionTypePaused)
		if existingCond == nil || existingCond.Status != metav1.ConditionTrue {
			tf.SetCondition(
				pause.ConditionTypePaused,
				metav1.ConditionTrue,
				"Paused",
				fmt.Sprintf("Reconciliation paused via %s annotation", PausedAnnotation),
			)

			if err := h.client.Status().Update(ctx, tf); err != nil {
				logger.Error(err, "failed to update paused condition")
			}
		}

		return ctrl.Result{}, nil
	}

	// Remove Paused condition only if it's currently set to True
	existingCond := tf.GetCondition(pause.ConditionTypePaused)
	if existingCond != nil && existingCond.Status == metav1.ConditionTrue {
		logger.Info("Transform unpaused, removing condition")
		tf.SetCondition(
			pause.ConditionTypePaused,
			metav1.ConditionFalse,
			"NotPaused",
			"Reconciliation is not paused",
		)

		if err := h.client.Status().Update(ctx, tf); err != nil {
			logger.Error(err, "failed to remove paused condition")
		}
	}

	// Update phase to Fetching
	tf.Status.Phase = platformv1alpha1.TransformPhaseFetching
	if err := h.client.Status().Update(ctx, tf); err != nil {
		logger.Error(err, "failed to update phase to Fetching")
	}

	// Step 4: Fetch CUE module and extract schema
	inputSchema, fetchResult, err := h.fetchAndExtractSchema(ctx, tf)
	if err != nil {
		tf.Status.Phase = platformv1alpha1.TransformPhaseFailed
		tf.SetCondition(
			"CueFetched",
			metav1.ConditionFalse,
			"FetchFailed",
			fmt.Sprintf("Failed to fetch/extract CUE schema: %v", err),
		)
		if statusErr := h.client.Status().Update(ctx, tf); statusErr != nil {
			logger.Error(statusErr, "failed to update status after fetch failure")
		}
		return ctrl.Result{}, err
	}

	// Update CueFetched condition
	tf.SetCondition(
		"CueFetched",
		metav1.ConditionTrue,
		"ModuleFetched",
		fmt.Sprintf("CUE module fetched from %s", fetchResult.Source),
	)

	// Update phase to Generating
	tf.Status.Phase = platformv1alpha1.TransformPhaseGenerating
	if err := h.client.Status().Update(ctx, tf); err != nil {
		logger.Error(err, "failed to update phase to Generating")
	}

	// Step 5: Generate and apply CRD
	generatedCRD, err := h.generateAndApplyCRD(ctx, tf, inputSchema)
	if err != nil {
		tf.Status.Phase = platformv1alpha1.TransformPhaseFailed
		tf.SetCondition(
			"CRDGenerated",
			metav1.ConditionFalse,
			"GenerationFailed",
			fmt.Sprintf("Failed to generate/apply CRD: %v", err),
		)
		if statusErr := h.client.Status().Update(ctx, tf); statusErr != nil {
			logger.Error(statusErr, "failed to update status after CRD generation failure")
		}
		return ctrl.Result{}, err
	}

	// Step 6: Update final status
	return h.updateStatus(ctx, tf, generatedCRD, fetchResult)
}

// fetchAndExtractSchema fetches the CUE module and extracts the input schema
func (h *TransformHandlers) fetchAndExtractSchema(
	ctx context.Context, tf *platformv1alpha1.Transform,
) (*apiextensionsv1.JSONSchemaProps, *platformloader.FetchResult, error) {
	logger := log.FromContext(ctx)

	var fetchResult *platformloader.FetchResult
	var cueValue cue.Value
	var err error

	// Build the fetch parameters
	var pullSecretRef *string
	if tf.Spec.CueRef.PullSecretRef != nil {
		pullSecretRef = &tf.Spec.CueRef.PullSecretRef.Name
	}

	switch tf.Spec.CueRef.Type {
	case platformv1alpha1.CueRefTypeEmbedded:
		// Load embedded module
		cueValue, err = h.loader.LoadEmbedded(tf.Spec.CueRef.Ref)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to load embedded CUE module: %w", err)
		}
		fetchResult = &platformloader.FetchResult{
			Digest: fmt.Sprintf("embedded:%s", tf.Spec.CueRef.Ref),
			Source: fmt.Sprintf("embedded://%s", tf.Spec.CueRef.Ref),
		}

	case platformv1alpha1.CueRefTypeInline:
		// Compile inline CUE
		cueValue = h.loader.Context().CompileString(tf.Spec.CueRef.Ref)
		if cueValue.Err() != nil {
			return nil, nil, fmt.Errorf("failed to compile inline CUE: %w", cueValue.Err())
		}
		fetchResult = &platformloader.FetchResult{
			Content: []byte(tf.Spec.CueRef.Ref),
			Digest:  platformloader.InlineType,
			Source:  platformloader.InlineType,
		}

	case platformv1alpha1.CueRefTypeOCI, platformv1alpha1.CueRefTypeGit, platformv1alpha1.CueRefTypeConfigMap:
		// Use fetcher system
		fetchResult, err = h.loader.FetchModule(ctx, string(tf.Spec.CueRef.Type), tf.Spec.CueRef.Ref, tf.Namespace, pullSecretRef)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to fetch CUE module: %w", err)
		}

		cueValue, err = h.loader.LoadFromContent(fetchResult.Content)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to compile fetched CUE module: %w", err)
		}

	default:
		return nil, nil, fmt.Errorf("unsupported CueRef type: %s", tf.Spec.CueRef.Type)
	}

	logger.Info("CUE module fetched successfully",
		"source", fetchResult.Source,
		"digest", fetchResult.Digest)

	// Extract the input schema from CUE
	inputSchema, err := h.extractor.ExtractInputSchema(cueValue)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to extract input schema: %w", err)
	}

	logger.Info("Input schema extracted successfully",
		"properties", len(inputSchema.Properties),
		"required", len(inputSchema.Required))

	return inputSchema, fetchResult, nil
}

// generateAndApplyCRD generates a CRD from the schema and applies it to the cluster
func (h *TransformHandlers) generateAndApplyCRD(
	ctx context.Context, tf *platformv1alpha1.Transform, inputSchema *apiextensionsv1.JSONSchemaProps,
) (*platformv1alpha1.GeneratedCRDReference, error) {
	logger := log.FromContext(ctx)

	// Build generator config from Transform spec
	config := crd.GeneratorConfig{
		Group:              tf.Spec.Group,
		Version:            tf.Spec.Version,
		ShortNames:         tf.Spec.ShortNames,
		Categories:         tf.Spec.Categories,
		TransformName:      tf.Name,
		TransformNamespace: tf.Namespace,
	}

	// Derive platform name from Transform name
	platformName := tf.Name

	// Generate the CRD
	generatedCRD := h.generator.GenerateCRD(platformName, inputSchema, config)

	logger.Info("Generated CRD",
		"name", generatedCRD.Name,
		"kind", generatedCRD.Spec.Names.Kind,
		"group", generatedCRD.Spec.Group)

	// Apply the CRD to the cluster
	if err := h.generator.ApplyCRD(ctx, h.client, generatedCRD); err != nil {
		return nil, fmt.Errorf("failed to apply CRD: %w", err)
	}

	logger.Info("CRD applied successfully", "name", generatedCRD.Name)

	// Record event
	if h.recorder != nil {
		h.recorder.Eventf(tf, "Normal", "CRDGenerated",
			"Generated and applied CRD %s (kind: %s)",
			generatedCRD.Name, generatedCRD.Spec.Names.Kind)
	}

	// Build the reference
	ref := &platformv1alpha1.GeneratedCRDReference{
		APIVersion: fmt.Sprintf("%s/%s", config.Group, config.Version),
		Kind:       generatedCRD.Spec.Names.Kind,
		Name:       generatedCRD.Name,
		Plural:     generatedCRD.Spec.Names.Plural,
	}

	// Apply defaults if not set
	if ref.APIVersion == "/" {
		ref.APIVersion = fmt.Sprintf("%s/%s", crd.DefaultGroup, crd.DefaultVersion)
	}

	return ref, nil
}

// updateStatus updates the Transform status with the generated CRD reference
func (h *TransformHandlers) updateStatus(
	ctx context.Context, tf *platformv1alpha1.Transform,
	generatedCRD *platformv1alpha1.GeneratedCRDReference, fetchResult *platformloader.FetchResult,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Update phase
	tf.Status.Phase = platformv1alpha1.TransformPhaseReady

	// Update GeneratedCRD reference
	tf.Status.GeneratedCRD = generatedCRD

	// Update ResolvedCueRef with fetch result
	if fetchResult != nil {
		now := metav1.Now()
		tf.Status.ResolvedCueRef = &platformv1alpha1.ResolvedCueReference{
			Digest:    fetchResult.Digest,
			FetchedAt: &now,
		}
	}

	// Set conditions
	tf.SetCondition(
		"CRDGenerated",
		metav1.ConditionTrue,
		"CRDApplied",
		fmt.Sprintf("CRD %s generated and applied successfully", generatedCRD.Name),
	)

	tf.SetCondition(
		"SchemaExtracted",
		metav1.ConditionTrue,
		"SchemaExtracted",
		"Input schema extracted from CUE module",
	)

	// Update observed generation
	tf.Status.ObservedGeneration = tf.Generation

	// Update status
	if err := h.client.Status().Update(ctx, tf); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update status: %w", err)
	}

	logger.Info("Transform reconciled successfully",
		"phase", tf.Status.Phase,
		"crd", generatedCRD.Name,
		"kind", generatedCRD.Kind)

	// Record event
	if h.recorder != nil {
		h.recorder.Eventf(tf, "Normal", "Ready",
			"Transform ready - CRD %s (kind: %s) is available for use",
			generatedCRD.Name, generatedCRD.Kind)
	}

	return ctrl.Result{}, nil
}

// handleDeletion handles Transform deletion by cleaning up and removing the finalizer
func (h *TransformHandlers) handleDeletion(ctx context.Context, tf *platformv1alpha1.Transform) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	if !controllerutil.ContainsFinalizer(tf, TransformFinalizer) {
		// Finalizer already removed, nothing to do
		return ctrl.Result{}, nil
	}

	logger.Info("Handling Transform deletion", "name", tf.Name)

	// Record deletion event
	if h.recorder != nil {
		h.recorder.Event(tf, "Normal", "Deleting", "Transform is being deleted")
	}

	// Delete the generated CRD if it exists
	if tf.Status.GeneratedCRD != nil {
		crdName := tf.Status.GeneratedCRD.Name
		logger.Info("Deleting generated CRD", "name", crdName)

		if err := h.generator.DeleteCRD(ctx, h.client, crdName); err != nil {
			if !errors.IsNotFound(err) {
				logger.Error(err, "Failed to delete generated CRD", "name", crdName)
				// Don't block deletion on CRD cleanup failure
			}
		} else {
			logger.Info("Deleted generated CRD", "name", crdName)
			if h.recorder != nil {
				h.recorder.Eventf(tf, "Normal", "CRDDeleted", "Deleted generated CRD %s", crdName)
			}
		}
	}

	// Remove finalizer to allow deletion to proceed
	logger.Info("Removing finalizer from Transform")
	controllerutil.RemoveFinalizer(tf, TransformFinalizer)
	if err := h.client.Update(ctx, tf); err != nil {
		logger.Error(err, "Failed to remove finalizer")
		return ctrl.Result{}, err
	}

	logger.Info("Transform deletion handled successfully")
	return ctrl.Result{}, nil
}
