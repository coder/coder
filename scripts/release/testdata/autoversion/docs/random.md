# Some documentation

1. Run the following command to install the chart in your cluster.

   For the **mainline** Coder release:

   <!-- autoversion(mainline): "--version [version] # trailing comment!" -->

   ```shell
   helm install coder coder-v2/coder \
       --namespace coder \
       --values values.yaml \
       --version 2.10.0 # trailing comment!
   ```
