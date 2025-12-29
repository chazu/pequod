#!/usr/bin/env bash
# Run Dagger CI pipeline locally or in CI
set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Function to print colored output
print_status() {
    echo -e "${GREEN}==>${NC} $1"
}

print_error() {
    echo -e "${RED}ERROR:${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}WARNING:${NC} $1"
}

# Check if dagger is installed
if ! command -v dagger &> /dev/null; then
    print_error "Dagger is not installed. Please install it first:"
    echo "  macOS: brew install dagger/tap/dagger"
    echo "  Linux: curl -L https://dl.dagger.io/dagger/install.sh | sh"
    echo "  Or visit: https://docs.dagger.io/install"
    exit 1
fi

# Get the command to run (default: ci)
COMMAND="${1:-ci}"

case "$COMMAND" in
    test)
        print_status "Running unit tests..."
        dagger call test --source=.
        ;;
    lint)
        print_status "Running linter..."
        dagger call lint --source=.
        ;;
    build)
        print_status "Building binary..."
        dagger call build --source=. export --path=./bin/manager
        print_status "Binary exported to ./bin/manager"
        ;;
    image)
        print_status "Building Docker image..."
        dagger call build-image --source=. --name=pequod-controller --tag=latest
        ;;
    e2e)
        print_status "Running E2E tests..."
        print_warning "This requires Docker to be running and will create a Kind cluster"
        dagger call e2e --source=.
        ;;
    ci)
        print_status "Running full CI pipeline..."
        dagger call ci --source=.
        ;;
    *)
        print_error "Unknown command: $COMMAND"
        echo "Usage: $0 [test|lint|build|image|e2e|ci]"
        echo ""
        echo "Commands:"
        echo "  test   - Run unit tests"
        echo "  lint   - Run golangci-lint"
        echo "  build  - Build the controller binary"
        echo "  image  - Build the Docker image"
        echo "  e2e    - Run E2E tests (requires Docker)"
        echo "  ci     - Run full CI pipeline (test + lint)"
        exit 1
        ;;
esac

print_status "Done!"

