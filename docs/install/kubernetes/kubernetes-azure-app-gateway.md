# Deploy Coder on Azure with an Application Gateway

In certain enterprise environments, the [Azure Application Gateway](https://learn.microsoft.com/en-us/azure/application-gateway/ingress-controller-overview) is required.

These steps serve as a proof-of-concept example so that you can get Coder running with Kubernetes on Azure. Your deployment might require a separate Postgres server or signed certificates.

The Application Gateway supports:

- Websocket traffic (required for workspace connections)
- TLS termination

Refer to Microsoft's documentation on how to [enable application gateway ingress controller add-on for an existing AKS cluster with an existing application gateway](https://learn.microsoft.com/en-us/azure/application-gateway/tutorial-ingress-controller-add-on-existing).
The steps here follow the Microsoft tutorial for a Coder deployment.

## Deploy Coder on Azure with an Application Gateway

1. Create Azure resource group:

   ```sql
   az group create --name myResourceGroup --location eastus
   ```

1. Create AKS cluster:

   ```sql
   az aks create --name myCluster --resource-group myResourceGroup --network-plugin azure --enable-managed-identity --generate-ssh-keys
   ```

1. Create public IP:

   ```sql
   az network public-ip create --name myPublicIp --resource-group myResourceGroup --allocation-method Static --sku Standard
   ```

1. Create VNet and subnet:

   ```sql
   az network vnet create --name myVnet --resource-group myResourceGroup --address-prefix 10.0.0.0/16 --subnet-name mySubnet --subnet-prefix 10.0.0.0/24
   ```

1. Create Azure application gateway, attach VNet, subnet and public IP:

   ```sql
   az network application-gateway create --name myApplicationGateway --resource-group myResourceGroup --sku Standard_v2 --public-ip-address myPublicIp --vnet-name myVnet --subnet mySubnet --priority 100
   ```

1. Get app gateway ID:

   ```sql
   appgwId=$(az network application-gateway show --name myApplicationGateway --resource-group myResourceGroup -o tsv --query "id")
   ```

1. Enable app gateway ingress to AKS cluster:

   ```sql
   az aks enable-addons --name myCluster --resource-group myResourceGroup --addon ingress-appgw --appgw-id $appgwId
   ```

1. Get AKS node resource group:

   ```sql
   nodeResourceGroup=$(az aks show --name myCluster --resource-group myResourceGroup -o tsv --query "nodeResourceGroup")
   ```

1. Get AKS VNet name:

   ```sql
   aksVnetName=$(az network vnet list --resource-group $nodeResourceGroup -o tsv --query "[0].name")
   ```

1. Get AKS VNet ID:

   ```sql
   aksVnetId=$(az network vnet show --name $aksVnetName --resource-group $nodeResourceGroup -o tsv --query "id")
   ```

1. Peer VNet to AKS VNet:

   ```sql
   az network vnet peering create --name AppGWtoAKSVnetPeering --resource-group myResourceGroup --vnet-name myVnet --remote-vnet $aksVnetId --allow-vnet-access
   ```

1. Get app gateway VNet ID:

   ```sql
   appGWVnetId=$(az network vnet show --name myVnet --resource-group myResourceGroup -o tsv --query "id")
   ```

1. Peer AKS VNet to app gateway VNet:

   ```sql
   az network vnet peering create --name AKStoAppGWVnetPeering --resource-group $nodeResourceGroup --vnet-name $aksVnetName --remote-vnet $appGWVnetId --allow-vnet-access
   ```

1. Get AKS credentials:

   ```sql
   az aks get-credentials --name myCluster --resource-group myResourceGroup
   ```

1. Create Coder namespace:

   ```shell
   kubectl create ns coder
   ```

1. Deploy non-production PostgreSQL instance to AKS cluster:

   ```shell
   helm repo add bitnami https://charts.bitnami.com/bitnami
   helm install coder-db bitnami/postgresql \
   --namespace coder \
   --set auth.username=coder \
   --set auth.password=coder \
   --set auth.database=coder \
   --set persistence.size=10Gi
   ```

1. Create the PostgreSQL secret:

   ```shell
   kubectl create secret generic coder-db-url -n coder --from-literal=url="postgres://coder:coder@coder-db-postgresql.coder.svc.cluster.local:5432/coder?sslmode=disable"
   ```

1. Deploy Coder to AKS cluster:

   ```shell
   helm repo add coder-v2 https://helm.coder.com/v2
   helm install coder coder-v2/coder \
       --namespace coder \
    --values values.yaml \
    --version 2.18.5
   ```

1. Clean up Azure resources:

   ```sql
   az group delete --name myResourceGroup
   az group delete --name MC_myResourceGroup_myCluster_eastus
   ```

1. Deploy the gateway - this needs clarification

1. After you deploy the gateway, add the following entries to Helm's `values.yaml` file before you deploy Coder:

   ```yaml
     service:
       enable: true
       type: ClusterIP
       sessionAffinity: None
       externalTrafficPolicy: Cluster
       loadBalancerIP: ""
       annotations: {}
       httpNodePort: ""
       httpsNodePort: ""

     ingress:
       enable: true
       className: "azure-application-gateway"
       host: ""
       wildcardHost: ""
       annotations: {}
       tls:
         enable: false
         secretName: ""
         wildcardSecretName: ""
   ```
