# Qualtive provider for OpenTofu and Terraform

> [!WARNING]
> **Alpha.** This provider is in early development. Every release is `0.x`, and resources, attributes and behaviour may change in any release. Pin an exact version.

Manage [Qualtive](https://qualtive.io) enquiries as code through the Qualtive Admin API.

- Registry address: `nightshift-habits/qualtive`
- Docs: [`docs/`](docs/) (also published on the OpenTofu and Terraform registries)

```hcl
terraform {
  required_providers {
    qualtive = {
      source  = "nightshift-habits/qualtive"
      version = "0.1.0"
    }
  }
}

provider "qualtive" {}  # reads QUALTIVE_API_KEY

resource "qualtive_enquiry" "contact" {
  container    = "acme"
  workspace_id = 17
  slug         = "contact"
  name         = { en = "Contact us" }
  pages = jsonencode([{
    id      = 1
    content = [{ type = "text", id = 1, storageTarget = { type = "text" } }]
  }])
}
```

## Authentication

Set `api_key` or `QUALTIVE_API_KEY` to a user API key (`u-…`). Its user needs the editor or admin role in every container you manage; container API keys (`c-…`) are read-only and can't create or edit enquiries.

> [!NOTE]
> Until the Admin API fix for API-key roles is deployed, a key acts one role below its user's role. So for now, writes need the key of a user with the **admin** role; an editor's key gets `403`. Roles are cached for up to 11 minutes, so an existing key may keep the old behaviour for that long after the fix is deployed.

## Recommended: `-parallelism=1`

The Admin API checks that a slug is free and then writes the enquiry, but the two steps aren't atomic. If two creates for the same slug run at the same time, both can succeed. Apply with `-parallelism=1`, and don't let two configurations manage the same container at the same time:

```sh
tofu apply -parallelism=1
```

## What's supported

| | |
|---|---|
| `qualtive_enquiry` resource | Identified by container and slug, imported with `<container>/<slug>`. Manages `slug`, `name`, `workspace_id`, `category_id`, `is_user_contact_details_required`, and `pages`, `submitted_pages` and `theme` as JSON in the Admin API format. |
| `qualtive_enquiry` data source | Looks up an enquiry by its current slug. |

Behaviour to know about:

- Create fails when the slug is already taken in the container. Import the existing enquiry instead; the provider never creates a suffixed `slug-2`.
- Changing `slug` renames the enquiry in place. Old slugs keep resolving for people posting feedback.
- Destroying an enquiry permanently deletes it. Its feedback entries are kept.
- Machine translations (`automaticTranslated*`) are ignored. Drafts made in the admin UI are left alone and reported through `has_draft`.
- Image content and other nested resources that need uploads aren't supported yet.
- Creating the same slug from two places at once can race. See [Recommended: `-parallelism=1`](#recommended--parallelism1).

## Development

Requires Go (see `go.mod`) and OpenTofu or Terraform.

```sh
make test       # unit tests, plus full plan/apply cycles against an in-memory fake Admin API
make lint       # golangci-lint
make generate   # format examples and regenerate docs/ with tfplugindocs
```

### Trying a local build

`scripts/dev` builds the provider into `bin/` and writes `.dev/terraform.rc` with `dev_overrides`, so OpenTofu and Terraform load the local build. Nothing outside the repo is touched.

```sh
export QUALTIVE_API_KEY=u-…
scripts/dev tofu -chdir=playground plan -var workspace_id=1
# or: eval "$(scripts/dev | tail -1)" and run tofu as usual
```

With `dev_overrides`, skip `tofu init` and ignore the warning about development overrides.

### Acceptance tests

Acceptance tests (`TestAcc*`) create and delete real enquiries. Run them against a local backend, never production:

```sh
TF_ACC=1 \
QUALTIVE_ENDPOINT=http://localhost:8093 \
QUALTIVE_API_KEY=u-… \
QUALTIVE_ACC_CONTAINER=qualtive \
QUALTIVE_ACC_WORKSPACE_ID=1 \
make testacc
```

The backend must include get-by-slug and `onSlugConflict` (`GET /feedback/questions/by-slug/{slug}`). The key's user needs the editor or admin role in `QUALTIVE_ACC_CONTAINER`; on a backend without the API-key role fix, use an admin's key. To run with OpenTofu, also set `TF_ACC_TERRAFORM_PATH=$(which tofu) TF_ACC_PROVIDER_HOST=registry.opentofu.org TF_ACC_PROVIDER_NAMESPACE=nightshift-habits`.

## Releasing

Versions are plain `vX.Y.Z` tags (no prerelease suffixes). Pushing a tag runs GoReleaser through `.github/workflows/release.yml`, which builds, signs the checksums with the release GPG key and publishes a GitHub release that both registries pick up.

```sh
git tag v0.1.0 && git push origin v0.1.0
```

## License

[MPL-2.0](LICENSE)
