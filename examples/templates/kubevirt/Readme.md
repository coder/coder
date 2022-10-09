- [Coder Iteration Loop](#org6df9caf)
  - [Start Coder](#org8a0efd5)
  - [coder url](#org11688e9)
- [kubevirt workspace](#org369d0e6)
  - [create template and cluster](#org59bbab0)
  - [update template and new cluster](#org939dfe1)
  - [grab new cluster kubeconfig](#org0e8b078)
  - [inner cluster](#orge2b4dcd)
  - [cni not yet working](#org204e816)



<a id="org6df9caf"></a>

# Coder Iteration Loop


<a id="org8a0efd5"></a>

## Start Coder

```tmate

cd ~/sharingio/coder
rm -rf ~/.config/coderv2/ # delete database
coder server --address=0.0.0.0:7080 --access-url=http://localhost:7080 --tunnel \
    2>&1 | tee coder-server.log
```

```shell
coder login `cat ~/.config/coderv2/url` -u ii -p ii -e ii@ii.nz
```

```
> Your Coder deployment hasn't been set up!

  Welcome to Coder, ii! You're authenticated.

  Get started by creating a template:  coder templates init
```


<a id="org11688e9"></a>

## coder url

```shell
grep "coder login https://" coder-server.log | cut -d\  -f 4
```

```
https://fcca6c2cae4534be6d63b1e72f9a5371.pit-1.try.coder.app
```


<a id="org369d0e6"></a>

# kubevirt workspace


<a id="org59bbab0"></a>

## create template and cluster

```tmate
cd ~/sharingio/coder
export CRI_PATH=/var/run/containerd/containerd.sock
export IMAGE_REPO=k8s.gcr.io
export NODE_VM_IMAGE_TEMPLATE=quay.io/capk/ubuntu-2004-container-disk:v1.22.0
coder template create kubevirt -d examples/templates/kubevirt --yes --parameter-file examples/templates/kubevirt/kubevirt.param.yaml
coder create kv1 --template kubevirt --parameter-file examples/templates/kubevirt/kubevirt.param.yaml --yes
```


<a id="org939dfe1"></a>

## update template and new cluster

```tmate
export WORKSPACE=kv1
coder template push kubevirt -d examples/templates/kubevirt --yes --parameter-file examples/templates/kubevirt/kubevirt.param.yaml
coder create $WORKSPACE --template kubevirt --parameter-file examples/templates/kubevirt/kubevirt.param.yaml --yes
```


<a id="org0e8b078"></a>

## grab new cluster kubeconfig

```tmate
export WORKSPACE=kv1
unset KUBECONFIG
TMPFILE=$(mktemp -t kubeconfig-XXXXX)
kubectl get secrets -n $WORKSPACE ${WORKSPACE}-kubeconfig  -o jsonpath={.data.value} | base64 -d > $TMPFILE
export KUBECONFIG=$TMPFILE
kubectl get ns
```


<a id="orge2b4dcd"></a>

## inner cluster

```shell
export WORKSPACE=kv1
unset KUBECONFIG
TMPFILE=$(mktemp -t kubeconfig-XXXXX)
kubectl get secrets -n $WORKSPACE ${WORKSPACE}-kubeconfig  -o jsonpath={.data.value} | base64 -d > $TMPFILE
export KUBECONFIG=$TMPFILE
kubectl get all -A
```

```
NAMESPACE     NAME                                    READY   STATUS    RESTARTS   AGE
default       pod/code-server-0                       0/1     Pending   0          81s
kube-system   pod/coredns-749558f7dd-mwwff            0/1     Pending   0          81s
kube-system   pod/coredns-749558f7dd-ppw92            0/1     Pending   0          81s
kube-system   pod/etcd-kv1-97525                      1/1     Running   0          90s
kube-system   pod/kube-apiserver-kv1-97525            1/1     Running   0          90s
kube-system   pod/kube-controller-manager-kv1-97525   1/1     Running   0          90s
kube-system   pod/kube-proxy-48s9l                    1/1     Running   0          81s
kube-system   pod/kube-scheduler-kv1-97525            1/1     Running   0          90s

NAMESPACE     NAME                 TYPE        CLUSTER-IP   EXTERNAL-IP   PORT(S)                  AGE
default       service/kubernetes   ClusterIP   10.95.0.1    <none>        443/TCP                  97s
kube-system   service/kube-dns     ClusterIP   10.95.0.10   <none>        53/UDP,53/TCP,9153/TCP   96s

NAMESPACE     NAME                        DESIRED   CURRENT   READY   UP-TO-DATE   AVAILABLE   NODE SELECTOR            AGE
kube-system   daemonset.apps/kube-proxy   1         1         1       1            1           kubernetes.io/os=linux   96s

NAMESPACE     NAME                      READY   UP-TO-DATE   AVAILABLE   AGE
kube-system   deployment.apps/coredns   0/2     2            0           96s

NAMESPACE     NAME                                 DESIRED   CURRENT   READY   AGE
kube-system   replicaset.apps/coredns-749558f7dd   2         2         0       82s

NAMESPACE   NAME                           READY   AGE
default     statefulset.apps/code-server   0/1     88s
```


<a id="org204e816"></a>

## cni not yet working

```shell
export WORKSPACE=kv1
unset KUBECONFIG
TMPFILE=$(mktemp -t kubeconfig-XXXXX)
kubectl get secrets -n $WORKSPACE ${WORKSPACE}-kubeconfig  -o jsonpath={.data.value} | base64 -d > $TMPFILE
export KUBECONFIG=$TMPFILE
kubectl describe nodes | grep -B6 KubeletNotReady
```

```
Conditions:
  Type             Status  LastHeartbeatTime                 LastTransitionTime                Reason                       Message
  ----             ------  -----------------                 ------------------                ------                       -------
  MemoryPressure   False   Sat, 08 Oct 2022 23:39:08 -0600   Sat, 08 Oct 2022 23:38:52 -0600   KubeletHasSufficientMemory   kubelet has sufficient memory available
  DiskPressure     False   Sat, 08 Oct 2022 23:39:08 -0600   Sat, 08 Oct 2022 23:38:52 -0600   KubeletHasNoDiskPressure     kubelet has no disk pressure
  PIDPressure      False   Sat, 08 Oct 2022 23:39:08 -0600   Sat, 08 Oct 2022 23:38:52 -0600   KubeletHasSufficientPID      kubelet has sufficient PID available
  Ready            False   Sat, 08 Oct 2022 23:39:08 -0600   Sat, 08 Oct 2022 23:38:52 -0600   KubeletNotReady              container runtime network not ready: NetworkReady=false reason:NetworkPluginNotReady message:Network plugin returns error: cni plugin not initialized
```
