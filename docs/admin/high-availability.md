# High Availability

- High Availability enables multiple instances of the Coder server connected to the same PostgreSQL database.

## Kubernetes Setup

- Using our Helm, just increase `coder.replicaCount` in `values.yaml`
- Custom Helm Chart:
  ```
  env:
    - name: POD_IP
      valueFrom:
        fieldRef:
          fieldPath: status.podIP
    - name: CODER_DERP_SERVER_RELAY_ADDRESS
      value: http://$(POD_IP)
  ```

## Virtual Machine Setup

Set `CODER_DERP_SERVER_RELAY_ADDRESS` to an address that other instances can access:
