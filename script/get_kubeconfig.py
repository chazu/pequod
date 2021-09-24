#!/usr/bin/env python
import ast
import getpass
import os
from pathlib import Path
import paramiko
from paramiko.client import SSHClient
import shutil
import subprocess
import sys
import time
import yaml

if __name__ == "__main__":


    cluster_name = input("new cluster name:")
    hostname = input("hostname:")
    username = input("username:")
    password = getpass.getpass("password (blank to use keypair auth:") or None

    home = str(Path.home())

    # SSH in and download the config file to a tempfile
    client = SSHClient()
    client.set_missing_host_key_policy(paramiko.AutoAddPolicy())

    client.load_system_host_keys()
    client.connect(hostname,
                   username=username,
                   password=password)

    stdin, stdout, stderr = client.exec_command("cat /etc/rancher/k3s/k3s.yaml")

    remote_config = "".join(stdout.readlines()).encode('utf-8').decode('unicode-escape')
    remote_config_loaded = yaml.safe_load(remote_config)

    local_config = open(f"{home}/.kube/config")
    local_config_loaded = yaml.safe_load(local_config)

    # Backup current kubeconfig
    now = time.time()
    shutil.copyfile(f"{home}/.kube/config", f"{home}/.kube/config-backup-{now}")

    # TODO If config exists, snip it out

    # splice relevant bits into kubeconfig
    new_config_user = remote_config_loaded['users'][0]
    new_config_context = remote_config_loaded['contexts'][0]
    new_config_cluster = remote_config_loaded['clusters'][0]

    new_cluster_name = hostname
    # Splice the hostname into the cluster
    new_config_cluster['cluster']['server'] = new_config_cluster['cluster']['server'].replace("127.0.0.1", hostname)

    # Set a better name
    new_config_cluster['name'] = cluster_name
    new_config_context['context']['cluster'] = cluster_name
    new_config_context['context']['user'] = cluster_name
    new_config_context['name'] = cluster_name

    # (Almost) ensure unique user name - no 'default' at least
    new_config_user['name'] = cluster_name

    local_config_loaded['clusters'].append(new_config_cluster)
    local_config_loaded['contexts'].append(new_config_context)
    local_config_loaded['users'].append(new_config_user)

    with open(f"{home}/.kube/config", 'w') as outfile:
        yaml.safe_dump(local_config_loaded, outfile, default_flow_style=False)
