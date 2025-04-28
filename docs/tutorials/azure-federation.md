# Federating Coder's control plane to Azure

<div>
  <a href="https://github.com/ericpaulsen" style="text-decoration: none; color: inherit;">
    <span style="vertical-align:middle;">Eric Paulsen</span>
    <img src="https://github.com/ericpaulsen.png" alt="ericpaulsen" width="24px" height="24px" style="vertical-align:middle; margin: 0px;"/>
  </a>
</div>
January 26, 2024

---

This guide will walkthrough how to authenticate a Coder Provisioner to Microsoft
Azure, using a Service Principal with a client certificate. You can use this
guide for authenticating Coder to Azure, regardless of where Coder is run,
either on-premise or in a non-Azure cloud. This method is one of several
[recommended by Terraform](https://registry.terraform.io/providers/hashicorp/azurerm/latest/docs#authenticating-to-azure).

## Step 1: Generate Client Certificate & PKCS bundle

We'll need to create the certificate Coder will use for authentication. Run the
below command to generate a private key and self-signed certificate:

```console
openssl req -subj '/CN=myclientcertificate/O=MyCompany, Inc./ST=CA/C=US' \
  -new -newkey rsa:4096 -sha256 -days 730 -nodes -x509 -keyout client.key -out client.crt
```

Next, generate a `.pfx` file to be used by Coder's Provisioner to authenticate
the AzureRM provider:

```console
openssl pkcs12 -export -password pass:"Pa55w0rd123" -out client.pfx -inkey client.key -in client.crt
```

## Step 2: Create Azure Application & Service Principal

Navigate to the Azure portal, and into the Microsoft Entra ID section. Select
the App Registration blade, and register a new application. Fill in the
following fields:

- **Name**: this is a friendly identifier and can be anything (e.g. "Coder")
- **Supported Account Types**: - set to "Accounts in this organizational
  directory only (single-tenant)"

The **Redirect URI** field does not need to be set in this case. Take note of
the `Application (client) ID` and `Directory (tenant) ID` values, which will be
used by Coder.

## Step 3: Assign Client Certificate to the Azure Application

To upload the certificate we created in Step 1, select **Certificates &
secrets** on the left-hand side, and select **Upload Certificate**. Upload the
public key file, which is `service-principal.crt` from the example above.

## Step 4: Set Permissions on the Service Principal

Now that the Application is created in Microsoft Entra ID, we need to assign
permissions to the Service Principal so it can provision Azure resources for
Coder users. Navigate to the Subscriptions blade in the Azure Portal, select the
**Subscription > Access Control (IAM) > Add > Add role assignment**.

Set the **Role** that grants the appropriate permissions to create the Azure
resources you need for your Coder workspaces. `Contributor` will provide
Read/Write on all Subscription resources. For more information on the available
roles, see the
[Microsoft documentation](https://learn.microsoft.com/en-us/azure/role-based-access-control/built-in-roles).

## Step 5: Configure Coder to use the Client Certificate

Now that the client certificate is uploaded to Azure, we need to mount the
certificate files into the Coder deployment. If running Coder on Kubernetes, you
will need to create the `.pfx` file as a Kubernetes secret, and mount it into
the Helm chart.

Run the below command to create the secret:

```console
kubectl create secret generic -n coder azure-client-cert-secret --from-file=client.pfx=/path/to/your/client.pfx
```

In addition, create secrets for each of the following values from your Azure
Application:

- Client ID
- Tenant ID
- Subscription ID
- Certificate password

Next, set the following values in Coder's Helm chart:

```yaml
coder:
  env:
    - name: ARM_CLIENT_ID
      valueFrom:
        secretKeyRef:
          key: id
          name: arm-client-id
    - name: ARM_CLIENT_CERTIFICATE_PATH
      value: /home/coder/az/
    - name: ARM_CLIENT_CERTIFICATE_PASSWORD
      valueFrom:
        secretKeyRef:
          key: password
          name: arm-client-cert-password
    - name: ARM_TENANT_ID
      valueFrom:
        secretKeyRef:
          key: id
          name: arm-tenant-id
    - name: ARM_SUBSCRIPTION_ID
      valueFrom:
        secretKeyRef:
          key: id
          name: arm-subscription-id
  volumes:
    - name: "azure-client-cert"
      secret:
        secretName: "azure-client-cert-secret"
  volumeMounts:
    - name: "azure-client-cert"
      mountPath: "/home/coder/az/"
      readOnly: true
```

Upgrade the Coder deployment using the following `helm` command:

```console
helm upgrade coder coder-v2/coder -n coder -f values.yaml
```
