#!/bin/bash

PASSHASH=`read newpass;echo -n $newpass | base64`
echo $PASSHASH

k8sec set argocd-initial-admin-secret password=$PASSHASH
