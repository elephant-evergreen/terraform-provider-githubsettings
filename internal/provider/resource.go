package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_            resource.ResourceWithConfigure        = (*policyResource)(nil)
	_            resource.ResourceWithImportState      = (*policyResource)(nil)
	_            resource.ResourceWithConfigValidators = (*policyResource)(nil)
	ownerPattern                                       = regexp.MustCompile(`^[A-Za-z0-9-]+$`)
	repoPattern                                        = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)
)

type policyResource struct {
	spec   policySpec
	client *gitHubClient
}

func (r *policyResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + r.spec.name
}

func (r *policyResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	identity := schema.StringAttribute{
		Required:      true,
		Description:   "GitHub account that owns the repository or organization.",
		PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
	}
	attributes := map[string]schema.Attribute{"owner": identity}
	if !r.spec.organization {
		repository := identity
		repository.Description = "GitHub repository name within the owner account."
		attributes["repository"] = repository
	}
	for _, field := range r.spec.fields {
		switch field.typ {
		case types.BoolType:
			attributes[field.name] = schema.BoolAttribute{Required: !field.computed, Computed: field.computed, Description: field.describe()}
		case types.StringType:
			attribute := schema.StringAttribute{Required: true, Description: field.describe()}
			if len(field.choices) > 0 {
				attribute.Validators = []validator.String{allowedValues(field.choices)}
			}
			attributes[field.name] = attribute
		case types.ListType{ElemType: types.StringType}:
			attributes[field.name] = schema.ListAttribute{Optional: true, ElementType: types.StringType, Description: field.describe()}
		default:
			attributes[field.name] = schema.SetAttribute{Optional: true, ElementType: types.Int64Type, Description: field.describe()}
		}
	}
	description := r.spec.description
	if description == "" {
		description = "Manages an existing GitHub policy. Removing the resource from State preserves the remote policy."
	}
	resp.Schema = schema.Schema{
		Description: description,
		Attributes:  attributes,
	}
}

func (r *policyResource) ConfigValidators(context.Context) []resource.ConfigValidator {
	return []resource.ConfigValidator{policyConfigValidator{spec: r.spec}}
}

func (r *policyResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	client, ok := req.ProviderData.(*gitHubClient)
	if !ok {
		resp.Diagnostics.AddError("Invalid GitHub provider data", fmt.Sprintf("Expected *gitHubClient, got %T.", req.ProviderData))
		return
	}
	r.client = client
}

type policyIdentity struct {
	owner      string
	repository string
}

func (spec policySpec) endpoint(identity policyIdentity) (string, error) {
	if !ownerPattern.MatchString(identity.owner) {
		return "", errors.New("owner must be a GitHub account name")
	}
	if spec.organization {
		if identity.repository != "" {
			return "", errors.New("organization policy imports use the organization name")
		}
		return "/orgs/" + identity.owner + spec.suffix, nil
	}
	if !repoPattern.MatchString(identity.repository) || identity.repository == "." || identity.repository == ".." {
		return "", errors.New("repository must be a GitHub repository name")
	}
	return "/repos/" + identity.owner + "/" + identity.repository + spec.suffix, nil
}

func (r *policyResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	owner, repo, hasRepo := strings.Cut(req.ID, "/")
	if hasRepo == r.spec.organization {
		resp.Diagnostics.AddError("Invalid GitHub policy import ID", "Use OWNER/REPOSITORY for a repository policy or OWNER for an organization policy.")
		return
	}
	if _, err := r.spec.endpoint(policyIdentity{owner: owner, repository: repo}); err != nil {
		resp.Diagnostics.AddError("Invalid GitHub policy import ID", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("owner"), owner)...)
	if hasRepo {
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("repository"), repo)...)
	}
}

func (r *policyResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	r.apply(ctx, req.Plan, &resp.State, &resp.Diagnostics)
}

func (r *policyResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	r.apply(ctx, req.Plan, &resp.State, &resp.Diagnostics)
}

func (r *policyResource) Delete(ctx context.Context, _ resource.DeleteRequest, resp *resource.DeleteResponse) {
	resp.State.RemoveResource(ctx)
}

func (r *policyResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var owner, repo types.String
	resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root("owner"), &owner)...)
	if !r.spec.organization {
		resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root("repository"), &repo)...)
	}
	if resp.Diagnostics.HasError() {
		return
	}
	result, err := r.read(ctx, policyIdentity{owner: owner.ValueString(), repository: repo.ValueString()})
	if err != nil {
		resp.Diagnostics.AddError("Could not read GitHub policy", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, result)...)
}

func (r *policyResource) apply(ctx context.Context, plan tfsdk.Plan, state *tfsdk.State, diagnostics *diag.Diagnostics) {
	var desired types.Object
	diagnostics.Append(plan.Get(ctx, &desired)...)
	if diagnostics.HasError() {
		return
	}
	values := desired.Attributes()
	identity := policyIdentity{owner: values["owner"].(types.String).ValueString()}
	if !r.spec.organization {
		identity.repository = values["repository"].(types.String).ValueString()
	}
	result, err := r.sync(ctx, identity, values)
	if err != nil {
		diagnostics.AddError("Could not apply GitHub policy", err.Error())
		return
	}
	diagnostics.Append(state.Set(ctx, result)...)
}

func (r *policyResource) matches(actual, desired map[string]attr.Value) bool {
	for _, field := range r.spec.fields {
		if !field.computed && !actual[field.name].Equal(desired[field.name]) {
			return false
		}
	}
	return true
}

func (r *policyResource) sync(ctx context.Context, identity policyIdentity, desired map[string]attr.Value) (types.Object, error) {
	actual, err := r.read(ctx, identity)
	if err != nil || r.matches(actual.Attributes(), desired) {
		return actual, err
	}
	if r.spec.readOnly {
		return types.ObjectNull(nil), errors.New("This resource records GitHub settings without writing them. Match the configuration to GitHub before applying.")
	}
	endpoint, err := r.spec.endpoint(identity)
	if err != nil {
		return types.ObjectNull(nil), err
	}
	data, err := r.payload(ctx, desired)
	if err != nil {
		return types.ObjectNull(nil), err
	}
	method, body, err := r.writeRequest(actual.Attributes(), desired, data)
	if err != nil {
		return types.ObjectNull(nil), err
	}
	if err := r.client.request(ctx, method, endpoint, body, nil); err != nil {
		return types.ObjectNull(nil), err
	}
	actual, err = r.read(ctx, identity)
	if err != nil {
		return types.ObjectNull(nil), err
	}
	if !r.matches(actual.Attributes(), desired) {
		return types.ObjectNull(nil), fmt.Errorf("GitHub did not report the requested values for %s", endpoint)
	}
	return actual, nil
}

func (r *policyResource) writeRequest(actual, desired map[string]attr.Value, data map[string]any) (string, any, error) {
	if r.spec.write != nil {
		return r.spec.write(actual, desired)
	}
	var body any = data
	for i := len(r.spec.jsonPath) - 1; i >= 0; i-- {
		body = map[string]any{r.spec.jsonPath[i]: body}
	}
	return r.spec.method, body, nil
}

func (r *policyResource) payload(ctx context.Context, values map[string]attr.Value) (map[string]any, error) {
	if r.spec.validate != nil {
		if err := r.spec.validate(ctx, values); err != nil {
			return nil, err
		}
	}
	result := make(map[string]any)
	for _, field := range r.spec.fields {
		if field.computed {
			continue
		}
		value := values[field.name]
		if value.IsUnknown() {
			return nil, fmt.Errorf("policy field %s is unknown", field.name)
		}
		switch value := value.(type) {
		case types.String:
			if value.IsNull() || !field.accepts(value.ValueString()) {
				return nil, fmt.Errorf("policy field %s has an invalid value", field.name)
			}
			result[field.name] = value.ValueString()
		case types.Bool:
			if value.IsNull() {
				return nil, fmt.Errorf("policy field %s is missing", field.name)
			}
			result[field.name] = value.ValueBool()
		case types.List:
			if !value.IsNull() {
				var keys []string
				if diags := value.ElementsAs(ctx, &keys, false); diags.HasError() {
					return nil, fmt.Errorf("could not read %s", field.name)
				}
				result[field.name] = append([]string{}, keys...)
			}
		case types.Set:
			if !value.IsNull() {
				var ids []int64
				if diags := value.ElementsAs(ctx, &ids, false); diags.HasError() {
					return nil, fmt.Errorf("could not read %s", field.name)
				}
				result[field.name] = append([]int64{}, ids...)
			}
		}
	}
	return result, nil
}

func (r *policyResource) read(ctx context.Context, identity policyIdentity) (types.Object, error) {
	if r.client == nil {
		return types.ObjectNull(nil), errors.New("GitHub provider is not configured")
	}
	endpoint, err := r.spec.endpoint(identity)
	if err != nil {
		return types.ObjectNull(nil), err
	}
	var reply map[string]json.RawMessage
	err = r.client.request(ctx, http.MethodGet, endpoint, nil, &reply)
	if err != nil && r.spec.readFallback != nil {
		reply, err = r.spec.readFallback(ctx, r.client, identity, err)
	}
	if err != nil {
		return types.ObjectNull(nil), err
	}

	for _, key := range r.spec.jsonPath {
		var nested map[string]json.RawMessage
		if json.Unmarshal(reply[key], &nested) != nil || nested == nil {
			return types.ObjectNull(nil), fmt.Errorf("GitHub did not return policy object %s", key)
		}
		reply = nested
	}
	fields := map[string]attr.Value{"owner": types.StringValue(identity.owner)}
	if !r.spec.organization {
		fields["repository"] = types.StringValue(identity.repository)
	}
	for _, field := range r.spec.fields {
		switch field.typ {
		case types.BoolType:
			var value *bool
			if json.Unmarshal(reply[field.name], &value) != nil || value == nil {
				return types.ObjectNull(nil), fmt.Errorf("GitHub did not return boolean policy field %s", field.name)
			}
			fields[field.name] = types.BoolValue(*value)
		case types.StringType:
			var value *string
			if json.Unmarshal(reply[field.name], &value) != nil || value == nil || !field.accepts(*value) {
				return types.ObjectNull(nil), fmt.Errorf("GitHub returned an unsupported value for policy field %s", field.name)
			}
			fields[field.name] = types.StringValue(*value)
		case types.ListType{ElemType: types.StringType}:
			var keys []string
			raw := reply[field.name]
			fields[field.name] = types.ListNull(types.StringType)
			if len(raw) != 0 && string(raw) != "null" {
				if json.Unmarshal(raw, &keys) != nil {
					return types.ObjectNull(nil), fmt.Errorf("GitHub did not return a string list for %s", field.name)
				}
				value, diags := types.ListValueFrom(ctx, types.StringType, keys)
				if diags.HasError() {
					return types.ObjectNull(nil), fmt.Errorf("could not represent %s", field.name)
				}
				fields[field.name] = value
			}
		default:
			fields[field.name] = types.SetNull(types.Int64Type)
		}
	}
	if r.spec.normalize != nil {
		if err := r.spec.normalize(ctx, r.client, endpoint, fields); err != nil {
			return types.ObjectNull(nil), err
		}
	}
	attributeTypes := make(map[string]attr.Type, len(fields))
	for name, value := range fields {
		attributeTypes[name] = value.Type(ctx)
	}
	result, diags := types.ObjectValue(attributeTypes, fields)
	if diags.HasError() {
		return types.ObjectNull(nil), errors.New("could not represent GitHub policy state")
	}
	return result, nil
}

type allowedValues []string

func (v allowedValues) Description(context.Context) string {
	return "Allowed values are " + strings.Join(v, ", ") + "."
}

func (v allowedValues) MarkdownDescription(ctx context.Context) string { return v.Description(ctx) }

func (v allowedValues) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if !req.ConfigValue.IsNull() && !req.ConfigValue.IsUnknown() && !slices.Contains(v, req.ConfigValue.ValueString()) {
		resp.Diagnostics.AddAttributeError(req.Path, "Unsupported policy value", v.Description(context.Background()))
	}
}

type policyConfigValidator struct{ spec policySpec }

func (v policyConfigValidator) Description(context.Context) string {
	return "Validates the GitHub policy target and selection."
}

func (v policyConfigValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v policyConfigValidator) ValidateResource(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var config types.Object
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() || config.IsNull() || config.IsUnknown() {
		return
	}
	resp.Diagnostics.Append(v.validate(ctx, config.Attributes())...)
}

func (v policyConfigValidator) validate(ctx context.Context, values map[string]attr.Value) diag.Diagnostics {
	var diagnostics diag.Diagnostics
	if err := v.spec.validateTarget(values); err != nil {
		diagnostics.AddError("Invalid GitHub policy target", err.Error())
	}
	if v.spec.validate != nil {
		if err := v.spec.validate(ctx, values); err != nil {
			attribute := "owner"
			var ruleErr *policyRuleError
			if errors.As(err, &ruleErr) {
				attribute = ruleErr.attribute
			}
			diagnostics.AddAttributeError(path.Root(attribute), "Invalid GitHub policy configuration", err.Error())
		}
	}
	return diagnostics
}

func (spec policySpec) validateTarget(values map[string]attr.Value) error {
	owner, ok := values["owner"].(types.String)
	if !ok || owner.IsUnknown() {
		return nil
	}
	identity := policyIdentity{owner: owner.ValueString()}
	if !spec.organization {
		repository, ok := values["repository"].(types.String)
		if !ok || repository.IsUnknown() {
			return nil
		}
		identity.repository = repository.ValueString()
	}
	_, err := spec.endpoint(identity)
	return err
}
