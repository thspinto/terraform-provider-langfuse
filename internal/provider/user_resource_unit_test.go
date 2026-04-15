package provider

import (
	"context"
	"fmt"
	"testing"

	"github.com/golang/mock/gomock"

	"github.com/langfuse/terraform-provider-langfuse/internal/langfuse"
	"github.com/langfuse/terraform-provider-langfuse/internal/langfuse/mocks"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	resschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

func TestUserResourceMetadata(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	r := NewUserResource()

	var resp resource.MetadataResponse
	r.Metadata(ctx, resource.MetadataRequest{ProviderTypeName: "langfuse"}, &resp)

	if resp.TypeName != "langfuse_user" {
		t.Fatalf("unexpected type name. got %q, want %q", resp.TypeName, "langfuse_user")
	}
}

func TestUserResourceSchema(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	r := NewUserResource()

	var schemaResp resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)

	if schemaResp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics from Schema: %v", schemaResp.Diagnostics)
	}
	if diags := schemaResp.Schema.ValidateImplementation(ctx); diags.HasError() {
		t.Fatalf("schema implementation validation failed: %v", diags)
	}

	idAttr, ok := schemaResp.Schema.Attributes["id"].(resschema.StringAttribute)
	if !ok || !idAttr.Computed {
		t.Fatalf("'id' must be a computed string attribute")
	}

	emailAttr, ok := schemaResp.Schema.Attributes["email"].(resschema.StringAttribute)
	if !ok || !emailAttr.Required {
		t.Fatalf("'email' must be a required string attribute")
	}

	activeAttr, ok := schemaResp.Schema.Attributes["active"].(resschema.BoolAttribute)
	if !ok || !activeAttr.Optional || !activeAttr.Computed {
		t.Fatalf("'active' must be an optional+computed bool attribute")
	}

	pubKeyAttr, ok := schemaResp.Schema.Attributes["organization_public_key"].(resschema.StringAttribute)
	if !ok || !pubKeyAttr.Required || !pubKeyAttr.Sensitive {
		t.Fatalf("'organization_public_key' must be a required sensitive string attribute")
	}

	privKeyAttr, ok := schemaResp.Schema.Attributes["organization_private_key"].(resschema.StringAttribute)
	if !ok || !privKeyAttr.Required || !privKeyAttr.Sensitive {
		t.Fatalf("'organization_private_key' must be a required sensitive string attribute")
	}

	ignoreDestroyAttr, ok := schemaResp.Schema.Attributes["ignore_destroy"].(resschema.BoolAttribute)
	if !ok || !ignoreDestroyAttr.Optional {
		t.Fatalf("'ignore_destroy' must be an optional bool attribute")
	}
}

func TestUserResourceCRUD(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()

	r, ok := NewUserResource().(*userResource)
	if !ok {
		t.Fatalf("NewUserResource did not return a *userResource")
	}

	clientFactory := mocks.NewMockClientFactory(ctrl)

	var resourceSchema resschema.Schema
	t.Run("Configure", func(t *testing.T) {
		var configureResp resource.ConfigureResponse
		r.Configure(ctx, resource.ConfigureRequest{ProviderData: clientFactory}, &configureResp)
		if configureResp.Diagnostics.HasError() {
			t.Fatalf("unexpected diagnostics from Configure: %v", configureResp.Diagnostics)
		}
		if r.ClientFactory == nil {
			t.Fatalf("ClientFactory is nil after Configure")
		}

		var schemaResp resource.SchemaResponse
		r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)
		if schemaResp.Diagnostics.HasError() {
			t.Fatalf("unexpected diagnostics from Schema: %v", schemaResp.Diagnostics)
		}
		resourceSchema = schemaResp.Schema
	})

	email := "test@example.com"
	userID := "user-123"
	pubKey := "pk-test"
	privKey := "sk-test"

	var createResp resource.CreateResponse
	t.Run("Create", func(t *testing.T) {
		clientFactory.OrganizationClient.EXPECT().CreateSCIMUser(ctx, gomock.Any()).Return(&langfuse.SCIMUserResponse{
			ID:       userID,
			UserName: email,
			Active:   true,
		}, nil)

		createPlan := tfsdk.Plan{
			Schema: resourceSchema,
			Raw: buildUserObjectValue(map[string]tftypes.Value{
				"id":                       tftypes.NewValue(tftypes.String, nil),
				"email":                    tftypes.NewValue(tftypes.String, email),
				"user_name":                tftypes.NewValue(tftypes.String, nil),
				"active":                   tftypes.NewValue(tftypes.Bool, nil),
				"organization_public_key":  tftypes.NewValue(tftypes.String, pubKey),
				"organization_private_key": tftypes.NewValue(tftypes.String, privKey),
				"ignore_destroy":           tftypes.NewValue(tftypes.Bool, nil),
			}),
		}
		createResp.State.Schema = resourceSchema
		r.Create(ctx, resource.CreateRequest{Plan: createPlan}, &createResp)
		if createResp.Diagnostics.HasError() {
			t.Fatalf("unexpected diagnostics from Create: %v", createResp.Diagnostics)
		}
	})

	var readResp resource.ReadResponse
	t.Run("Read", func(t *testing.T) {
		clientFactory.OrganizationClient.EXPECT().FindSCIMUserByEmail(ctx, email).Return(&langfuse.SCIMUserResponse{
			ID:       userID,
			UserName: email,
			Active:   true,
			Emails: []struct {
				Value   string `json:"value"`
				Primary bool   `json:"primary"`
			}{
				{Value: email, Primary: true},
			},
		}, nil)

		readResp.State.Schema = resourceSchema
		r.Read(ctx, resource.ReadRequest{State: createResp.State}, &readResp)
		if readResp.Diagnostics.HasError() {
			t.Fatalf("unexpected diagnostics from Read: %v", readResp.Diagnostics)
		}
	})

	var updateResp resource.UpdateResponse
	t.Run("Update", func(t *testing.T) {
		clientFactory.OrganizationClient.EXPECT().UpdateSCIMUser(ctx, userID, gomock.Any()).Return(&langfuse.SCIMUserResponse{
			ID:       userID,
			UserName: email,
			Active:   false,
		}, nil)

		updatePlan := tfsdk.Plan{
			Schema: resourceSchema,
			Raw: buildUserObjectValue(map[string]tftypes.Value{
				"id":                       tftypes.NewValue(tftypes.String, userID),
				"email":                    tftypes.NewValue(tftypes.String, email),
				"user_name":                tftypes.NewValue(tftypes.String, email),
				"active":                   tftypes.NewValue(tftypes.Bool, false),
				"organization_public_key":  tftypes.NewValue(tftypes.String, pubKey),
				"organization_private_key": tftypes.NewValue(tftypes.String, privKey),
				"ignore_destroy":           tftypes.NewValue(tftypes.Bool, nil),
			}),
		}
		updateResp.State.Schema = resourceSchema
		r.Update(ctx, resource.UpdateRequest{Plan: updatePlan, State: readResp.State}, &updateResp)
		if updateResp.Diagnostics.HasError() {
			t.Fatalf("unexpected diagnostics from Update: %v", updateResp.Diagnostics)
		}
	})

	t.Run("Delete", func(t *testing.T) {
		clientFactory.OrganizationClient.EXPECT().DeleteSCIMUser(ctx, userID).Return(nil)

		var deleteResp resource.DeleteResponse
		deleteResp.State.Schema = resourceSchema
		r.Delete(ctx, resource.DeleteRequest{State: updateResp.State}, &deleteResp)
		if deleteResp.Diagnostics.HasError() {
			t.Fatalf("unexpected diagnostics from Delete: %v", deleteResp.Diagnostics)
		}
	})
}

func TestUserResource_Read_NotFound(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()

	r := NewUserResource().(*userResource)
	clientFactory := mocks.NewMockClientFactory(ctrl)
	r.ClientFactory = clientFactory

	var schemaResp resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)
	resourceSchema := schemaResp.Schema

	email := "missing@example.com"
	clientFactory.OrganizationClient.EXPECT().FindSCIMUserByEmail(ctx, email).Return(nil, fmt.Errorf("cannot find user with email %q", email))

	state := tfsdk.State{
		Schema: resourceSchema,
		Raw: buildUserObjectValue(map[string]tftypes.Value{
			"id":                       tftypes.NewValue(tftypes.String, "user-gone"),
			"email":                    tftypes.NewValue(tftypes.String, email),
			"user_name":                tftypes.NewValue(tftypes.String, email),
			"active":                   tftypes.NewValue(tftypes.Bool, true),
			"organization_public_key":  tftypes.NewValue(tftypes.String, "pk"),
			"organization_private_key": tftypes.NewValue(tftypes.String, "sk"),
			"ignore_destroy":           tftypes.NewValue(tftypes.Bool, nil),
		}),
	}

	var readResp resource.ReadResponse
	readResp.State.Schema = resourceSchema
	r.Read(ctx, resource.ReadRequest{State: state}, &readResp)

	if readResp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", readResp.Diagnostics)
	}
	if !readResp.State.Raw.IsNull() {
		t.Fatalf("expected state to be removed (null) for not-found user, got: %v", readResp.State.Raw)
	}
}

func TestUserResource_Read_Error(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()

	r := NewUserResource().(*userResource)
	clientFactory := mocks.NewMockClientFactory(ctrl)
	r.ClientFactory = clientFactory

	var schemaResp resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)
	resourceSchema := schemaResp.Schema

	email := "error@example.com"
	clientFactory.OrganizationClient.EXPECT().FindSCIMUserByEmail(ctx, email).Return(nil, fmt.Errorf("internal server error"))

	state := tfsdk.State{
		Schema: resourceSchema,
		Raw: buildUserObjectValue(map[string]tftypes.Value{
			"id":                       tftypes.NewValue(tftypes.String, "user-123"),
			"email":                    tftypes.NewValue(tftypes.String, email),
			"user_name":                tftypes.NewValue(tftypes.String, email),
			"active":                   tftypes.NewValue(tftypes.Bool, true),
			"organization_public_key":  tftypes.NewValue(tftypes.String, "pk"),
			"organization_private_key": tftypes.NewValue(tftypes.String, "sk"),
			"ignore_destroy":           tftypes.NewValue(tftypes.Bool, nil),
		}),
	}

	var readResp resource.ReadResponse
	readResp.State.Schema = resourceSchema
	r.Read(ctx, resource.ReadRequest{State: state}, &readResp)

	if !readResp.Diagnostics.HasError() {
		t.Fatal("expected diagnostics error for API error, but got none")
	}
}

func TestUserResource_Delete_IgnoreDestroy(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()

	r := NewUserResource().(*userResource)
	clientFactory := mocks.NewMockClientFactory(ctrl)
	r.ClientFactory = clientFactory

	// No DeleteSCIMUser call expected when ignore_destroy=true

	var schemaResp resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)
	resourceSchema := schemaResp.Schema

	state := tfsdk.State{
		Schema: resourceSchema,
		Raw: buildUserObjectValue(map[string]tftypes.Value{
			"id":                       tftypes.NewValue(tftypes.String, "user-123"),
			"email":                    tftypes.NewValue(tftypes.String, "test@example.com"),
			"user_name":                tftypes.NewValue(tftypes.String, "test@example.com"),
			"active":                   tftypes.NewValue(tftypes.Bool, true),
			"organization_public_key":  tftypes.NewValue(tftypes.String, "pk"),
			"organization_private_key": tftypes.NewValue(tftypes.String, "sk"),
			"ignore_destroy":           tftypes.NewValue(tftypes.Bool, true),
		}),
	}

	var deleteResp resource.DeleteResponse
	deleteResp.State.Schema = resourceSchema
	r.Delete(ctx, resource.DeleteRequest{State: state}, &deleteResp)

	if deleteResp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics from Delete with ignore_destroy=true: %v", deleteResp.Diagnostics)
	}
}

func buildUserObjectValue(values map[string]tftypes.Value) tftypes.Value {
	return tftypes.NewValue(
		tftypes.Object{
			AttributeTypes: map[string]tftypes.Type{
				"id":                       tftypes.String,
				"email":                    tftypes.String,
				"user_name":                tftypes.String,
				"active":                   tftypes.Bool,
				"organization_public_key":  tftypes.String,
				"organization_private_key": tftypes.String,
				"ignore_destroy":           tftypes.Bool,
			},
			OptionalAttributes: map[string]struct{}{
				"id":             {},
				"user_name":      {},
				"active":         {},
				"ignore_destroy": {},
			},
		},
		values,
	)
}
