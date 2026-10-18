package provider

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"testing"

	fwresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/hashicorp/terraform-plugin-testing/compare"
	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
	"github.com/nightshift-habits/terraform-provider-qualtive/internal/client"
)

const enquiryAddress = "qualtive_enquiry.test"

func TestEnquiryResourceLifecycle(t *testing.T) {
	testEnquiryLifecycle(t, newFakeEnv(t))
}

func TestAccEnquiryResourceLifecycle(t *testing.T) {
	testEnquiryLifecycle(t, newAccEnv(t))
}

func TestEnquiryResourceSlugConflict(t *testing.T) {
	testEnquirySlugConflict(t, newFakeEnv(t))
}

func TestAccEnquiryResourceSlugConflict(t *testing.T) {
	testEnquirySlugConflict(t, newAccEnv(t))
}

func TestEnquiryResourceRevertsOutsideChanges(t *testing.T) {
	env := newFakeEnv(t)
	slug := randomSlug()

	env.run(t, resource.TestCase{
		ProtoV6ProviderFactories: testProtoV6ProviderFactories,
		CheckDestroy:             env.checkEnquiriesDestroyed,
		Steps: []resource.TestStep{
			{
				Config: env.enquiryConfig(slug, `{ en = "Contact us" }`, ""),
			},
			{
				PreConfig: func() {
					env.fake.mutate(env.container, slug, func(enquiry *fakeEnquiry) {
						enquiry.Slug = slug + "-renamed"
						enquiry.Name = map[string]string{"en": "Renamed in the admin UI"}
					})
				},
				Config: env.enquiryConfig(slug, `{ en = "Contact us" }`, ""),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(enquiryAddress, plancheck.ResourceActionUpdate),
					},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(enquiryAddress, tfjsonpath.New("slug"), knownvalue.StringExact(slug)),
					statecheck.ExpectKnownValue(enquiryAddress, tfjsonpath.New("name"), knownvalue.MapExact(map[string]knownvalue.Check{
						"en": knownvalue.StringExact("Contact us"),
					})),
				},
			},
			{
				PreConfig: func() {
					env.fake.remove(env.container, slug)
				},
				Config: env.enquiryConfig(slug, `{ en = "Contact us" }`, ""),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(enquiryAddress, plancheck.ResourceActionCreate),
					},
				},
			},
		},
	})
}

func TestEnquiryResourceRejectsInvalidSlug(t *testing.T) {
	env := newFakeEnv(t)

	env.run(t, resource.TestCase{
		ProtoV6ProviderFactories: testProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      env.enquiryConfig("Contact Us", `{ en = "Contact us" }`, ""),
				ExpectError: regexp.MustCompile(`must start with a lowercase letter or digit`),
			},
		},
	})
}

func TestEnquiryResourceImportID(t *testing.T) {
	tests := map[string]struct {
		id        string
		container string
		slug      string
		wantError bool
	}{
		"container and slug": {id: "qualtive/hur-fungerar-admin", container: "qualtive", slug: "hur-fungerar-admin"},
		"missing container":  {id: "/contact", wantError: true},
		"missing slug":       {id: "qualtive/", wantError: true},
		"only slug":          {id: "contact", wantError: true},
		"invalid slug":       {id: "qualtive/Contact Us", wantError: true},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			r := &enquiryResource{}
			var schemaResp fwresource.SchemaResponse
			r.Schema(ctx, fwresource.SchemaRequest{}, &schemaResp)
			resp := fwresource.ImportStateResponse{State: tfsdk.State{
				Schema: schemaResp.Schema,
				Raw:    tftypes.NewValue(schemaResp.Schema.Type().TerraformType(ctx), nil),
			}}

			r.ImportState(ctx, fwresource.ImportStateRequest{ID: test.id}, &resp)

			if test.wantError {
				if !resp.Diagnostics.HasError() {
					t.Fatal("expected an error")
				}
				return
			}
			if resp.Diagnostics.HasError() {
				t.Fatal(resp.Diagnostics)
			}
			var model enquiryModel
			resp.Diagnostics.Append(resp.State.Get(ctx, &model)...)
			if model.Container.ValueString() != test.container || model.Slug.ValueString() != test.slug || !model.ID.IsNull() {
				t.Errorf("got container %q, slug %q, id %s", model.Container.ValueString(), model.Slug.ValueString(), model.ID)
			}
		})
	}
}

func testEnquiryLifecycle(t *testing.T, env *testEnv) {
	slug := randomSlug()
	renamedSlug := slug + "-renamed"
	sameID := statecheck.CompareValue(compare.ValuesSame())

	env.run(t, resource.TestCase{
		ProtoV6ProviderFactories: testProtoV6ProviderFactories,
		CheckDestroy:             env.checkEnquiriesDestroyed,
		Steps: []resource.TestStep{
			{
				Config: env.enquiryConfig(slug, `{ en = "Contact us" }`, ""),
				ConfigStateChecks: []statecheck.StateCheck{
					sameID.AddStateValue(enquiryAddress, tfjsonpath.New("id")),
					statecheck.ExpectKnownValue(enquiryAddress, tfjsonpath.New("id"), knownvalue.StringRegexp(regexp.MustCompile(`^\d+$`))),
					statecheck.ExpectKnownValue(enquiryAddress, tfjsonpath.New("slug"), knownvalue.StringExact(slug)),
					statecheck.ExpectKnownValue(enquiryAddress, tfjsonpath.New("workspace_id"), knownvalue.Int64Exact(env.workspaceID)),
					statecheck.ExpectKnownValue(enquiryAddress, tfjsonpath.New("category_id"), knownvalue.Null()),
					statecheck.ExpectKnownValue(enquiryAddress, tfjsonpath.New("submitted_pages"), knownvalue.StringExact(defaultSubmittedPages)),
					statecheck.ExpectKnownValue(enquiryAddress, tfjsonpath.New("theme"), knownvalue.StringExact(defaultTheme)),
					statecheck.ExpectKnownValue(enquiryAddress, tfjsonpath.New("is_user_contact_details_required"), knownvalue.Bool(false)),
					statecheck.ExpectKnownValue(enquiryAddress, tfjsonpath.New("has_draft"), knownvalue.Bool(false)),
				},
			},
			{
				ResourceName:      enquiryAddress,
				ImportState:       true,
				ImportStateId:     env.container + "/" + slug,
				ImportStateVerify: true,
			},
			{
				Config: env.enquiryConfig(renamedSlug, `{ en = "Contact", sv = "Kontakt" }`, `
  is_user_contact_details_required = true
  submitted_pages = jsonencode([{
    id         = 1
    conditions = []
    content    = [{ type = "title", id = 1, text = { en = "Thanks!" } }]
  }])
  theme = jsonencode({
    background                                   = { type = "predefined", value = "plain" }
    font                                         = { type = "predefined", value = "default" }
    cornerStyle                                  = "square"
    isBackgroundAttachmentVisibleInUserResponses = false
    isBackgroundColorVisibleInUserResponses      = false
  })
`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(enquiryAddress, plancheck.ResourceActionUpdate),
					},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					sameID.AddStateValue(enquiryAddress, tfjsonpath.New("id")),
					statecheck.ExpectKnownValue(enquiryAddress, tfjsonpath.New("slug"), knownvalue.StringExact(renamedSlug)),
					statecheck.ExpectKnownValue(enquiryAddress, tfjsonpath.New("name"), knownvalue.MapExact(map[string]knownvalue.Check{
						"en": knownvalue.StringExact("Contact"),
						"sv": knownvalue.StringExact("Kontakt"),
					})),
					statecheck.ExpectKnownValue(enquiryAddress, tfjsonpath.New("is_user_contact_details_required"), knownvalue.Bool(true)),
				},
			},
			{
				ResourceName:      enquiryAddress,
				ImportState:       true,
				ImportStateId:     env.container + "/" + renamedSlug,
				ImportStateVerify: true,
			},
			{
				Config: env.enquiryConfig(renamedSlug, `{ en = "Contact", sv = "Kontakt" }`, `
  is_user_contact_details_required = true
  submitted_pages = jsonencode([{
    id         = 1
    conditions = []
    content    = [{ type = "title", id = 1, text = { en = "Thanks!" } }]
  }])
  theme = jsonencode({
    background                                   = { type = "predefined", value = "plain" }
    font                                         = { type = "predefined", value = "default" }
    cornerStyle                                  = "square"
    isBackgroundAttachmentVisibleInUserResponses = false
    isBackgroundColorVisibleInUserResponses      = false
  })
`) + fmt.Sprintf(`
data "qualtive_enquiry" "test" {
  container    = qualtive_enquiry.test.container
  slug         = qualtive_enquiry.test.slug
  workspace_id = %d
}
`, env.workspaceID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair("data.qualtive_enquiry.test", "id", enquiryAddress, "id"),
					resource.TestCheckResourceAttrPair("data.qualtive_enquiry.test", "name.%", enquiryAddress, "name.%"),
					resource.TestCheckResourceAttrPair("data.qualtive_enquiry.test", "name.sv", enquiryAddress, "name.sv"),
					resource.TestCheckResourceAttrPair("data.qualtive_enquiry.test", "pages", enquiryAddress, "pages"),
					resource.TestCheckResourceAttrPair("data.qualtive_enquiry.test", "submitted_pages", enquiryAddress, "submitted_pages"),
					resource.TestCheckResourceAttrPair("data.qualtive_enquiry.test", "theme", enquiryAddress, "theme"),
					resource.TestCheckResourceAttr("data.qualtive_enquiry.test", "is_user_contact_details_required", "true"),
					resource.TestCheckResourceAttr("data.qualtive_enquiry.test", "has_draft", "false"),
				),
			},
		},
	})
}

func testEnquirySlugConflict(t *testing.T, env *testEnv) {
	slug := randomSlug()

	env.run(t, resource.TestCase{
		ProtoV6ProviderFactories: testProtoV6ProviderFactories,
		CheckDestroy:             env.checkEnquiriesDestroyed,
		Steps: []resource.TestStep{
			{
				Config: env.enquiryConfig(slug, `{ en = "First" }`, "") + fmt.Sprintf(`
resource "qualtive_enquiry" "duplicate" {
  container    = %q
  workspace_id = %d
  slug         = qualtive_enquiry.test.slug
  name         = { en = "Second" }
  pages        = qualtive_enquiry.test.pages
}
`, env.container, env.workspaceID),
				ExpectError: regexp.MustCompile(`Slug already exists`),
			},
		},
	})
}

func (env *testEnv) enquiryConfig(slug, name, extra string) string {
	return env.providerBlock + fmt.Sprintf(`
resource "qualtive_enquiry" "test" {
  container    = %q
  workspace_id = %d
  slug         = %q
  name         = %s
  pages = jsonencode([{
    id = 1
    content = [
      { type = "title", id = 1, text = { en = "Contact us" } },
      { type = "text", id = 2, storageTarget = { type = "text" } },
    ]
  }])
%s}
`, env.container, env.workspaceID, slug, name, extra)
}

func (env *testEnv) checkEnquiriesDestroyed(state *terraform.State) error {
	for _, rs := range state.RootModule().Resources {
		if rs.Type != "qualtive_enquiry" {
			continue
		}
		id, err := strconv.ParseInt(rs.Primary.ID, 10, 64)
		if err != nil {
			return err
		}
		_, err = env.client.GetEnquiry(context.Background(), rs.Primary.Attributes["container"], id)
		if !errors.Is(err, client.ErrNotFound) {
			return fmt.Errorf("enquiry %d still exists (err: %v)", id, err)
		}
	}
	return nil
}

func randomSlug() string {
	return "tf-acc-" + acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)
}
