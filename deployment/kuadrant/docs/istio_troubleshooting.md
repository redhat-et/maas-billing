# Troubleshooting Istio

If you installed Istio using the upstream helm chart it must be fully purged in order to use the Openshift Service Mesh operator.

## Full Purge of Istio

-----

### Using `istioctl` (Recommended)

This is the most common and effective way to uninstall Istio and all its components. The `--purge` flag is essential for removing cluster-wide resources.

1.  **Remove all Istio resources and CRDs**:

    ```bash
    istioctl uninstall --purge -y
    ```

      - The `--purge` flag removes all Istio-related resources, including the CRDs that `istioctl` installed.
      - The `-y` flag skips the confirmation prompt.

2.  **Manually delete the Istio namespace**: `istioctl uninstall` doesn't remove the namespace itself.

    ```bash
    kubectl delete namespace istio-system
    ```

3.  **Check for lingering resources**: After the above steps, run these commands to ensure everything is gone.

    ```bash
    kubectl get crds | grep istio
    kubectl get mutatingwebhookconfigurations | grep istio
    kubectl get validatingwebhookconfigurations | grep istio
    ```

    The output should be empty. If you find any remaining resources, you may need to delete them manually using `kubectl delete`.

-----

### Using Helm

If you installed Istio using Helm, you must use Helm to uninstall it.

1.  **List your Istio Helm charts**:

    ```bash
    helm ls -n istio-system
    ```

    This will show you the names of the Istio charts you need to uninstall (e.g., `istio-base`, `istiod`, etc.).

2.  **Uninstall the charts**: Uninstall the charts in the correct order: first the control plane (`istiod`), then the base chart.

    ```bash
    helm uninstall <your-istiod-chart-name> -n istio-system
    helm uninstall <your-base-chart-name> -n istio-system
    ```

3.  **Manually remove CRDs**: A key drawback of uninstalling with Helm is that it **does not remove the CRDs**. You must do this manually.

    ```bash
    kubectl delete crd $(kubectl get crd -A | grep "istio.io" | awk '{print $1}')
    ```

4.  **Remove the namespace**:

    ```bash
    kubectl delete namespace istio-system
    ```

-----

### General Cleanup Steps

Regardless of the installation method, you should perform these final cleanup steps:

1.  **Unlabel namespaces**: Remove any `istio-injection` or `istio.io/rev` labels from namespaces to prevent new pods from getting sidecars injected.

    ```bash
    kubectl get namespace --show-labels
    ```

    For each namespace with an Istio label, run:

    ```bash
    kubectl label namespace <your-namespace> istio-injection-
    # or
    kubectl label namespace <your-namespace> istio.io/rev-
    ```

2. **Check for leftover sidecars**: If you still have pods with the Envoy sidecar, you must restart or re-deploy them after Istio is uninstalled. The sidecar will be removed on the next deployment.

3.  **Remove sample applications**: If you installed any sample applications (like Bookinfo), remember to delete them from their respective namespaces.