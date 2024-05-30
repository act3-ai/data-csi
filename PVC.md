# Supporting Persistent Volume Claims

The external provisioner, by design, does not have a mechanism to do this.  The maintainers want to keep CSI more isolated from the internals of k8s.

Here is a [pull request](https://github.com/kubernetes-csi/external-provisioner/pull/425/files) bringing in the labels and annotations from the PVC over to the parameters when CreateVolume is called.  It was rejected.
