package provider

import (
	"context"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
)

type settingsProvider struct {
	version string
}

func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &settingsProvider{version: version}
	}
}

func (p *settingsProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "githubsettings"
	resp.Version = p.version
}

func (p *settingsProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description:         "Manages GitHub repository and organization policies using GITHUB_TOKEN.",
		MarkdownDescription: "Manages GitHub repository and organization policies through the GitHub REST API.\n\nSet `GITHUB_TOKEN` to a token with access to the configured repositories or organization. The provider has no configuration arguments; Terraform reads the token from the environment.\n\nResources manage existing GitHub settings; the provider does not create repositories or organizations. Destroying a resource removes it from Terraform state and stops management without changing the GitHub setting.",
	}
}

func (p *settingsProvider) Configure(_ context.Context, _ provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		resp.Diagnostics.AddError("Missing GitHub credentials", "Set GITHUB_TOKEN to a token with access to the configured repositories or organization.")
		return
	}
	resp.ResourceData = newGitHubClient(token)
}

func (p *settingsProvider) Resources(context.Context) []func() resource.Resource {
	factories := make([]func() resource.Resource, 0, len(policies))
	for _, policy := range policies {
		factories = append(factories, func() resource.Resource { return &policyResource{spec: policy} })
	}
	return factories
}

func (p *settingsProvider) DataSources(context.Context) []func() datasource.DataSource {
	return nil
}
