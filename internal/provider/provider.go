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
		Description:         "Manage GitHub Actions, pull request, release, and security settings for existing repositories and organizations.",
		MarkdownDescription: "Manage GitHub Actions, pull request, release, and security settings for existing repositories and organizations. Destroying a resource removes it from Terraform state and leaves the GitHub setting in place.",
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
