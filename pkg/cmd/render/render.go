package render

import (
	"fmt"
	"io/ioutil"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"k8s.io/klog/v2"

	configv1 "github.com/openshift/api/config/v1"
	kubecloudconfig "github.com/openshift/cluster-config-operator/pkg/operator/kube_cloud_config"
	genericrender "github.com/openshift/library-go/pkg/operator/render"
	genericrenderoptions "github.com/openshift/library-go/pkg/operator/render/options"
)

// renderOpts holds values to drive the render command.
type renderOpts struct {
	manifest genericrenderoptions.ManifestOptions
	generic  genericrenderoptions.GenericOptions

	clusterConfigFile string

	clusterInfrastructureInputFile string
	cloudProviderConfigInputFile   string
	cloudProviderConfigOutputFile  string
}

// NewRenderCommand creates a render command.
func NewRenderCommand() *cobra.Command {
	renderOpts := renderOpts{
		generic:  *genericrenderoptions.NewGenericOptions(),
		manifest: *genericrenderoptions.NewManifestOptions("config", "openshift/origin-cluster-config-operator:latest"),
	}
	cmd := &cobra.Command{
		Use:   "render",
		Short: "Render kubernetes API server bootstrap manifests, secrets and configMaps",
		Run: func(cmd *cobra.Command, args []string) {
			if err := renderOpts.Validate(); err != nil {
				klog.Fatal(err)
			}
			if err := renderOpts.Complete(); err != nil {
				klog.Fatal(err)
			}
			if err := renderOpts.Run(); err != nil {
				klog.Fatal(err)
			}
		},
	}

	renderOpts.AddFlags(cmd.Flags())

	return cmd
}

func (r *renderOpts) AddFlags(fs *pflag.FlagSet) {
	r.manifest.AddFlags(fs, "config")
	r.generic.AddFlags(fs, configv1.GroupVersion.WithKind("Config"))

	fs.StringVar(&r.clusterConfigFile, "cluster-config-file", r.clusterConfigFile, "Openshift Cluster API Config file.")

	// This is the file containing the infrastructure object
	fs.StringVar(&r.clusterInfrastructureInputFile, "cluster-infrastructure-input-file", r.clusterInfrastructureInputFile, "Input path for the cluster infrastructure file.")

	// This is the file containing the configmap for the kube cloud config provided by the user
	fs.StringVar(&r.cloudProviderConfigInputFile, "cloud-provider-config-input-file", r.cloudProviderConfigInputFile, "Input path for the cloud provider config file.")

	// This is the generated kube cloud config
	fs.StringVar(&r.cloudProviderConfigOutputFile, "cloud-provider-config-output-file", r.cloudProviderConfigOutputFile, "Output path for the generated cloud provider config file.")

}

// Validate verifies the inputs.
func (r *renderOpts) Validate() error {
	if err := r.manifest.Validate(); err != nil {
		return err
	}
	if err := r.generic.Validate(); err != nil {
		return err
	}

	// Validate all files are specified when specifying infrastructure and configmap files
	if infra, provider := len(r.clusterInfrastructureInputFile) != 0, len(r.cloudProviderConfigOutputFile) != 0; infra || provider {
		if !(infra && provider) {
			return fmt.Errorf("clulster-infrastructure-file and cloud-provider-config-output-file must be specified.")
		}
		if infra {
			if err := kubecloudconfig.ValidateFile(r.clusterInfrastructureInputFile); err != nil {
				return err
			}
		}
	}

	return nil
}

// Complete fills in missing values before command execution.
func (r *renderOpts) Complete() error {
	if err := r.manifest.Complete(); err != nil {
		return err
	}
	if err := r.generic.Complete(); err != nil {
		return err
	}
	return nil
}

type TemplateData struct {
	genericrenderoptions.ManifestConfig
	genericrenderoptions.FileConfig
}

// Run contains the logic of the render command.
func (r *renderOpts) Run() error {
	renderConfig := TemplateData{}

	if len(r.clusterConfigFile) > 0 {
		_, err := ioutil.ReadFile(r.clusterConfigFile)
		if err != nil {
			return err
		}
		// TODO I'm thinking we parse this into a map and reference it that way
	}

	if err := r.manifest.ApplyTo(&renderConfig.ManifestConfig); err != nil {
		return err
	}
	if err := r.generic.ApplyTo(
		&renderConfig.FileConfig,
		genericrenderoptions.Template{},
		genericrenderoptions.Template{},
		&renderConfig,
		nil,
	); err != nil {
		return err
	}

	if err := genericrender.WriteFiles(&r.generic, &renderConfig.FileConfig, renderConfig); err != nil {
		return err
	}

	if len(r.clusterInfrastructureInputFile) > 0 && len(r.cloudProviderConfigOutputFile) > 0 {
		targetCloudConfigMapData, err := kubecloudconfig.BootstrapTransform(r.clusterInfrastructureInputFile, r.cloudProviderConfigInputFile)
		if err != nil {
			return err
		}
		if err := ioutil.WriteFile(r.cloudProviderConfigOutputFile, targetCloudConfigMapData, 0644); err != nil {
			return fmt.Errorf("failed to write merged config to %q: %v", r.cloudProviderConfigOutputFile, err)
		}
	}

	return nil
}
