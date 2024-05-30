> :warning: This is prerelease software.

# Data Bottle CSI Driver

CSI driver that uses a data bottle as a volume.

Currently the driver makes use of `ace/data/tool` to download the dataset if it is not already available, launch a new instance of it named after the volumeHandle, and mount it.

This driver supports up to and including `data.act3-ace.io/v1` bottles.

## Usage

### Build the CSI driver

```bash
make build
```

### Installing into Kubernetes

```bash
helm install charts/csi-bottle csi-bottle
```

### Example Usage in Kubernetes

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: nginx
spec:
  containers:
  - name: nginx
    image: nginx:1.13-alpine
    ports:
    - containerPort: 80
    volumeMounts:
    - name: data
      mountPath: /usr/share/nginx/html
  volumes:
  - name: data
    csi:
      driver: bottle.csi.act3-ace.io
      # Add a secret to pull the bottle (if auth is required by the registry)
      # nodePublishSecretRef:
      #   name: test-secret
      volumeAttributes:
        # Specify the bottle to pull
        bottle: us-central1-docker.pkg.dev/aw-df16163b-7044-4662-93fa-ec0/public-down-auth-up/mnist:v1.6
        # or by digest
        # bottle: bottle:sha256:1234...
        
        # Optionally select what subset of data you want to pull down
        # selector: "subset=train,component=image|type=usage"
```

## Developing

### Start Image driver manually

```bash
sudo csi-bottle serve --endpoint tcp://127.0.0.1:10000 --nodeid CSINode -v 2
```

#### With Podman

This is **dangerous** because you have to run the driver as root and it needs to be able to run mount.  This is only possible when running in privileged mode so mounts happen on the host.

```bash
podman run --privileged=true --rm -it -p 127.0.0.1:10000:10000 reg.git.act3-ace.com/ace/data/csi --endpoint tcp://0.0.0.0:10000 --nodeid CSINode -v 2
```

### With Vagrant

This is the preferred way to test because the mount command only impacts the guest OS.

```bash
make vagrant
```

Along the way you inspect the directories with `make vagrant-tree`

### Test with an ACE Telemetry Server

The [ACE Telemetry Project](https://gitlab.com/act3-ai/asce/data/telemetry) can be run locally to test pull events with `make run ACE_DATA_CSI_TELEMETRY=http://ip-of-host:8100`.  To test with telemetry with Vagrant run:

```shell
make vagrant EXTRA_ARGS="--telemetry https://telemetry.lion.act3-ace.ai"
```

### Debugging with Vagrant

```shell
vagrant ssh -c 'sudo /opt/go/bin/dlv exec /vagrant/bin/csi-bottle-linux-amd64 --headless --api-version 2 -l 0.0.0.0:39000 -- serve --endpoint tcp://0.0.0.0:10000 --nodeid CSINode -v 4'
```

Then add this to your ~/.vscode/launch.json`

```json
{
    // Use IntelliSense to learn about possible attributes.
    // Hover to view descriptions of existing attributes.
    // For more information, visit: https://go.microsoft.com/fwlink/?linkid=830387
    "version": "0.2.0",
    "configurations": [
        {
            "name": "Connect to vagrant",
            "type": "go",
            "request": "attach",
            "mode": "remote",
            "remotePath": "${workspaceFolder}",
            "port": 39000,
            "host": "127.0.0.1"
        }
    ]
}
```

TODO hitting a breakpoint seems to kill the executable.  So this approach is not useful for debugging just yet.

### Setup

`make tool` will download and install essential tools.

Create a `.envrc` file for use with `direnv` and add the following

```shell
# shellcheck shell=bash
export KUBECONFIG=$PWD/.kubeconfig.yaml
PATH_add bin
PATH_add ci-bin
PATH_add tool
```

### Test using csc

[csc](https://github.com/rexray/gocsi/tree/master/csc) is helpful for testing the CSI driver.

#### Mount a volume

Run the following:

```bash
# The extra coma is needed.  The format is "mode,type[,fstype,mntflags]"
CAP=SINGLE_NODE_WRITER,mount,
VOLCON="--vol-context bottle=us-central1-docker.pkg.dev/aw-df16163b-7044-4662-93fa-ec0/public-down-auth-up/mnist:v1.6 --vol-context selector=subset=train"
```

For an example by bottle ID (to test the telemetry server)

```shell
VOLCON="--vol-context bottle=bottle:sha256:8d90d933cffe2c82c383e1a2ecd6da700fc714a9634144dd7a822a1d77432566 --vol-context selector=subset=train"
```

#### Regular volume

```bash
# mount
csc -e tcp://127.0.0.1:10000 node stage myvolume --with-spec-validation $VOLCON --cap $CAP --staging-target-path /tmp/csi/staging/somepod
csc -e tcp://127.0.0.1:10000 node publish myvolume --with-spec-validation $VOLCON --cap $CAP --staging-target-path /tmp/csi/staging/somepod --target-path /tmp/csi/target/somepod
# un-mount
csc -e tcp://127.0.0.1:10000 node unpublish myvolume --with-spec-validation --target-path /tmp/csi/target/somepod
csc -e tcp://127.0.0.1:10000 node unstage myvolume --with-spec-validation --staging-target-path /tmp/csi/staging/somepod
```

#### Ephemeral volume

Stage and unstage are not called for ephemeral volumes.

```shell
csc -e tcp://127.0.0.1:10000 node publish evolume --with-spec-validation $VOLCON --vol-context csi.storage.k8s.io/ephemeral=true --cap $CAP --target-path /tmp/csi/target/somepod
csc -e tcp://127.0.0.1:10000 node unpublish evolume --with-spec-validation --target-path /tmp/csi/target/somepod
```

#### Ephemeral Volume with Auth

Assuming you are already logged in using a credential helper we can extract the credentials with:

```shell
echo "reg.git.act3-ace.com" | docker-credential-secretservice get | jq -r '(.Username)+":"+(.Secret) | @base64 as $CREDS | {"auths": {"reg.git.act3-ace.com": {"auth": $CREDS }}}' > test-auth.json

VOLCON="--vol-context bottle=reg.git.act3-ace.com/ace/data/tool/bottle/mnist:v1.6"
export X_CSI_SECRETS="\".dockerconfigjson=$(sed 's/"/\"\"/g' test-auth.json)\""
csc -e tcp://127.0.0.1:10000 node publish evolume --with-spec-validation $VOLCON --vol-context csi.storage.k8s.io/ephemeral=true --cap $CAP --target-path /tmp/csi/target/somepod
csc -e tcp://127.0.0.1:10000 node unpublish evolume --with-spec-validation --target-path /tmp/csi/target/somepod
```

### Testing with csi-sanity

[Tutorial](https://kubernetes.io/blog/2020/01/08/testing-of-csi-drivers/)

Run the battery of tests:

```bash
csi-sanity -csi.endpoint dns:///127.0.0.1:10000 -csi.testvolumeparameters test-vol-params.yaml -csi.checkpathcmd ./checkpath.sh
```

To test with auth you need to create the test-secrets.yaml file.

```shell
dockerconfigjson="$(cat test-auth.json)" yq e -n "(.NodePublishVolumeSecret.\".dockerconfigjson\" = strenv(dockerconfigjson)) | (.NodeStageVolumeSecret.\".dockerconfigjson\" = strenv(dockerconfigjson))" > test-secrets.yaml
```

```bash
csi-sanity -csi.endpoint dns:///127.0.0.1:10000 -csi.testvolumeparameters test-vol-params-auth.yaml -csi.secrets test-secrets.yaml -csi.checkpathcmd ./checkpath.sh
```

## End to End Testing

### Using KinD

Create a cluster, deploy, and verify

```shell
kind create cluster --config kind.yaml
skaffold run
kubectl get csidriver,csinodes,pods
```

#### Test Ephemeral/Inline Volumes

##### Without Auth

```shell
kubectl apply -f examples/pod.yaml # or helm test csi-bottle
kubectl exec -it test-csi -- ls -l /var/www/html
kubectl delete -f examples/pod.yaml
```

##### With Auth

```shell
kubectl create secret docker-registry test-secret --docker-server=reg.git.act3-ace.com --docker-username=DOCKER_USERNAME --docker-password=DOCKER_PASSWORD
kubectl apply -f examples/pod-auth.yaml
kubectl exec -it test-csi-auth -- ls -l /var/www/html
kubectl delete -f examples/pod-auth.yaml
```

#### Persistent volumes (not fully implemented yet)

```shell
kubectl apply -f examples/persistent
kubectl get pvc,pv,pods
```

## Multipass

This is an alternative to Vagrant.

On Linux, `multipass` is installed as a snap which means it can only access your home directory.  So your source code must be in your home directory (if it is not you can bind mount it in).

```shell
multipass launch -n csi-bottle -d 20G --cloud-init cloud-config.yaml
multipass transfer bin/csi-bottle-linux-amd64 csi-bottle:/home/ubuntu/bin/csi-bottle
multipass exec csi-bottle -- sudo /home/ubuntu/bin/csi-bottle serve --endpoint tcp://0.0.0.0:10000 --nodeid CSINode -v=4
```

```shell
# find the VM's IPv4 address
MULTIPASS_IP=$(multipass info csi-bottle --format json | jq -r '.info."csi-bottle".ipv4[0]')

socat TCP-LISTEN:10000,fork TCP:$MULTIPASS_IP:10000
multipass exec csi-bottle -- sudo tree -ha /tmp/csi
```

---

Approved for public release: distribution unlimited. Case Number: AFRL-2024-2616
