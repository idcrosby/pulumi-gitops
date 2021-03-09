package main

import (
	"github.com/pulumi/pulumi-gcp/sdk/v4/go/gcp/container"
	"github.com/pulumi/pulumi-gcp/sdk/v4/go/gcp/serviceAccount"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v2/go/kubernetes/core/v1"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v2/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi-kubernetes/sdk/v2/go/kubernetes/providers"
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

		return nil
	})
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
