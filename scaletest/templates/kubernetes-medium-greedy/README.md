# kubernetes-medium-greedy

Provisions a medium-sized workspace with no persistent storage. Greedy agent variant.

_Note_: It is assumed you will be running workspaces on a dedicated GKE nodepool.
By default, this template sets a node affinity of `cloud.google.com/gke-nodepool` = `big-workspaces`.
The nodepool affinity can be customized with the variable `kubernetes_nodepool_workspaces`.
