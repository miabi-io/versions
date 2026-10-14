# Miabi versions

Static release manifests served from **https://versions.miabi.io**. Miabi's update check and the
`miabi` CLI read them to learn the latest release of each component, and which component versions a
Miabi release was tested with.

Components: `miabi`, `agent`, `runner`, `gateway`, `cli`.

## Endpoints

| URL | Contents |
|---|---|
| `/v1/index.json` | Every component in one file (what Miabi fetches) |
| `/v1/<component>/latest.json` | One component |
| `/schema/v1.json` | JSON Schema for both |

```json
{
  "$schema": "https://versions.miabi.io/schema/v1.json",
  "component": "miabi",
  "stable": {
    "version": "1.10.18",
    "released_at": "2026-10-13T18:46:14Z",
    "release_url": "https://github.com/miabi-io/miabi/releases/tag/v1.10.18",
    "reason": "bugfix",
    "image": "miabi/miabi:1.10.18",
    "components": { "agent": "0.6.2", "cli": "0.20.0", "gateway": "1.1.1", "runner": "0.2.0" }
  }
}
```

- `stable` and `prerelease` are channels. Only a build that is itself a prerelease should read
  `prerelease`. It is dropped automatically once a stable release reaches it.
- `version` has no leading `v` (Docker tag style).
- `reason` is `bugfix`, `feature`, `security` or `other`. Consumers show `security` prominently.
- `components` (miabi only) is the bundle the release was tested with. Goma Gateway is offered at
  this version, not Goma's own latest.

**Contract for consumers.** The data is advisory: it may drive a notice, never an automatic
upgrade or a compatibility floor. Requests send nothing identifying beyond `User-Agent`. Replay the
`ETag`. Paths under `/v1/` never change incompatibly; a breaking change ships as `/v2/`.

## Publishing a release

Add this as the last step of a component's release job, after its images are pushed:

```yaml
- uses: miabi-io/versions@main
  with:
    component: agent                      # miabi | agent | runner | cli
    version: ${{ github.ref_name }}       # a "-" suffix publishes to the prerelease channel
    reason: bugfix                        # bugfix | feature | security | other
    token: ${{ secrets.VERSIONS_TOKEN }}
```

Miabi's workflow also publishes its tested bundle, and the gateway it pins in the same pull request
(`also`; an older gateway than the published one is skipped):

```yaml
- uses: miabi-io/versions@main
  with:
    component: miabi
    version: ${{ github.ref_name }}
    components: gateway=1.1.1,agent=0.6.2,runner=0.2.0,cli=0.20.0
    also: gateway=${{ env.GOMA_VERSION }}
    token: ${{ secrets.VERSIONS_TOKEN }}
```

The action updates the manifest, regenerates `v1/index.json`, and opens a pull request set to
auto-merge once **Validate** passes. Re-running a release is a no-op. An older version than the
one published is refused.

## Local use

```bash
make test
make validate
go run ./cmd/versions bump -component cli -version v0.21.0 -reason feature   # GOWORK=off inside the monorepo
make index
```

Never edit `v1/index.json` by hand. CI fails when it doesn't match the component files.