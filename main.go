package main

import (
	"github.com/pulumi/pulumi-gcp/sdk/v4/go/gcp/container"
	serviceaccount "github.com/pulumi/pulumi-gcp/sdk/v4/go/gcp/serviceAccount"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v2/go/kubernetes/core/v1"
	"github.com/pulumi/pulumi-kubernetes/sdk/v2/go/kubernetes/helm/v3"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v2/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi-kubernetes/sdk/v2/go/kubernetes/providers"
	"github.com/pulumi/pulumi-kubernetes/sdk/v2/go/kubernetes/yaml"
	"github.com/pulumi/pulumi/sdk/v2/go/pulumi"
)

func main() {
	pulumi.Run(func(ctx *pulumi.Context) error {
		defaultSA, err := serviceaccount.NewAccount(ctx, "_default", &serviceaccount.AccountArgs{
			AccountId:   pulumi.String("service-account-id"),
			DisplayName: pulumi.String("Service Account"),
		})
		if err != nil {
			return err
		}
		gitOps, err := container.NewCluster(ctx, "gitops", &container.ClusterArgs{
			Location:              pulumi.String("us-east1"),
			RemoveDefaultNodePool: pulumi.Bool(true),
			InitialNodeCount:      pulumi.Int(1),
		})
		if err != nil {
			return err
		}
		_, err = container.NewNodePool(ctx, "gitops-reemptiblenodes", &container.NodePoolArgs{
			Location:  pulumi.String("us-east1"),
			Cluster:   gitOps.Name,
			NodeCount: pulumi.Int(1),
			NodeConfig: &container.NodePoolNodeConfigArgs{
				Preemptible:    pulumi.Bool(true),
				MachineType:    pulumi.String("e2-medium"),
				ServiceAccount: defaultSA.Email,
				OauthScopes: pulumi.StringArray{
					pulumi.String("https://www.googleapis.com/auth/cloud-platform"),
				},
			},
		})
		if err != nil {
			return err
		}

		ctx.Export("kubeconfig", generateKubeconfig(gitOps.Endpoint, gitOps.Name, gitOps.MasterAuth))

		k8sProvider, err := providers.NewProvider(ctx, "k8sprovider", &providers.ProviderArgs{
			Kubeconfig: generateKubeconfig(gitOps.Endpoint, gitOps.Name, gitOps.MasterAuth),
		}, pulumi.DependsOn([]pulumi.Resource{gitOps}))
		if err != nil {
			return err
		}

		_, err = corev1.NewNamespace(ctx, "pulumi-test", &corev1.NamespaceArgs{
			Metadata: &metav1.ObjectMetaArgs{
				Name: pulumi.String("pulumi-test-ns"),
			},
		}, pulumi.Provider(k8sProvider))
		if err != nil {
			return err
		}

		err = deployIngress(ctx)
		if err != nil {
			return err
		}

		err = deployArgo(ctx, k8sProvider)
		if err != nil {
			return err
		}

		return nil
	})
}

func deployIngress(ctx *pulumi.Context) error {

	_, err := helm.NewChart(ctx, "ingress-nginx", helm.ChartArgs{
		Chart:     pulumi.String("ingress-nginx"),
		Namespace: pulumi.String("ingress-nginx"),
		FetchArgs: helm.FetchArgs{
			Repo: pulumi.String("https://kubernetes.github.io/ingress-nginx"),
		},
	})
	if err != nil {
		return err
	}
	return nil
}

func deployArgo(ctx *pulumi.Context, provider pulumi.ProviderResource) error {
	namespace := "argocd"
	_, err := corev1.NewNamespace(ctx, namespace, &corev1.NamespaceArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name: pulumi.String(namespace),
		},
	}, pulumi.Provider(provider))

	_, err = helm.NewChart(ctx, "argocd", helm.ChartArgs{
		Namespace: pulumi.String(namespace),
		Chart:     pulumi.String("argo-cd"),
		FetchArgs: helm.FetchArgs{
			Repo: pulumi.String("https://argoproj.github.io/argo-helm"),
		},
		Values: pulumi.Map{
			"installCRDs": pulumi.Bool(false),
			"server": pulumi.Map{
				"service": pulumi.Map{
					"type": pulumi.String("LoadBalancer"),
				},
			},
		},
		// The helm chart is using a deprecated apiVersion,
		// So let's transform it
		Transformations: []yaml.Transformation{
			func(state map[string]interface{}, opts ...pulumi.ResourceOption) {
				if state["apiVersion"] == "extensions/v1beta1" {
					state["apiVersion"] = "networking.k8s.io/v1beta1"
				}
			},
		},
	})
	if err != nil {
		return err
	}
	return nil
}

func setupRBAC() {

}

func setupManagedDatabases() {

}

func clusterAutoscaler() {

}

func deployOperators() {

}

func generateKubeconfig(clusterEndpoint pulumi.StringOutput, clusterName pulumi.StringOutput,
	clusterMasterAuth container.ClusterMasterAuthOutput) pulumi.StringOutput {
	context := pulumi.Sprintf("pulumi_%s", clusterName)

	return pulumi.Sprintf(`apiVersion: v1
clusters:
- cluster:
    certificate-authority-data: %s
    server: https://%s
  name: %s
contexts:
- context:
    cluster: %s
    user: %s
  name: %s
current-context: %s
kind: Config
preferences: {}
users:
- name: %s
  user:
    auth-provider:
      config:
        cmd-args: config config-helper --format=json
        cmd-path: gcloud
        expiry-key: '{.credential.token_expiry}'
        token-key: '{.credential.access_token}'
      name: gcp`,
		clusterMasterAuth.ClusterCaCertificate().Elem(),
		clusterEndpoint, context, context, context, context, context, context)
}
