package provider

import (
	"context"
	"errors"
	"net/http"
	"regexp"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var claimPattern = regexp.MustCompile(`^[A-Za-z0-9_]+$`)

var repositoryOIDCSubjectTemplatePolicy = policySpec{
	name:        repositoryOIDCSubjectTemplate,
	suffix:      "/actions/oidc/customization/sub",
	method:      http.MethodPut,
	description: "Manages the OpenID Connect subject template for GitHub Actions in one repository. Creating or updating this resource applies the configured template to GitHub; destroying it removes the resource from Terraform state without changing the template. GitHub's REST API requires fine-grained repository Actions read access to read this setting on a private repository and Actions write access to update it. Changing the subject format can require corresponding changes to the cloud provider's trust policy; see https://docs.github.com/en/actions/concepts/security/openid-connect. API reference: https://docs.github.com/en/rest/actions/oidc#set-the-customization-template-for-an-oidc-subject-claim-for-a-repository.",
	fields: []policyField{
		flag("use_default", "Use GitHub's default subject claim template."),
		flag("use_immutable_subject", "Use an immutable subject claim."),
		{name: "include_claim_keys", typ: types.ListType{ElemType: types.StringType}, description: "Ordered claim keys to include in the subject. Omit when use_default is true."},
	},
}

func oidcSubjectTemplateRule(ctx context.Context, values map[string]attr.Value) error {
	useDefault, ok := values["use_default"].(types.Bool)
	if !ok || useDefault.IsNull() || useDefault.IsUnknown() {
		return nil
	}
	keys, ok := values["include_claim_keys"].(types.List)
	if !ok || keys.IsUnknown() {
		return nil
	}
	if useDefault.ValueBool() {
		if !keys.IsNull() {
			return &policyRuleError{attribute: "include_claim_keys", message: "Omit include_claim_keys when use_default is true."}
		}
		return nil
	}
	if keys.IsNull() {
		return nil
	}
	elements := keys.Elements()
	seen := make(map[string]bool, len(elements))
	for _, element := range elements {
		key, ok := element.(types.String)
		if !ok || key.IsNull() {
			return &policyRuleError{attribute: "include_claim_keys", message: "include_claim_keys must contain distinct alphanumeric or underscore names."}
		}
		if key.IsUnknown() {
			continue
		}
		name := key.ValueString()
		if !claimPattern.MatchString(name) || seen[name] {
			return &policyRuleError{attribute: "include_claim_keys", message: "include_claim_keys must contain distinct alphanumeric or underscore names."}
		}
		seen[name] = true
	}
	return nil
}

func normalizeOIDCSubjectTemplate(values map[string]attr.Value) error {
	useDefault, ok := values["use_default"].(types.Bool)
	if !ok || useDefault.IsNull() || useDefault.IsUnknown() {
		return errors.New("GitHub did not return use_default for the OIDC subject template")
	}
	if _, ok := values["include_claim_keys"].(types.List); !ok {
		return errors.New("could not represent include_claim_keys")
	}
	if useDefault.ValueBool() {
		values["include_claim_keys"] = types.ListNull(types.StringType)
	}
	return nil
}
