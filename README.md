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

Miabi's workflow also records its tested bundle. Every bundled component newer than its published
version is published in the same pull request, so a Miabi release that moves to a new gateway also
moves `gateway.stable`; one that is not newer is left as it is:

```yaml
- uses: miabi-io/versions@main
  with:
    component: miabi
    version: ${{ github.ref_name }}
    components: gateway=1.1.2,agent=0.6.2,runner=0.2.0,cli=0.20.3
    token: ${{ secrets.VERSIONS_TOKEN }}
```

`also` publishes other components released alongside without adding them to the bundle, with the
same rule.

The action updates the manifest, regenerates `v1/index.json`, and opens a pull request; the
repository merges it once **Validate** passes. The release job does not wait for the merge.
Re-running a release is a no-op. An older version than the one published is refused.

## Local use

```bash
make test
make validate
go run ./cmd/versions bump -component cli -version v0.21.0 -reason feature   # GOWORK=off inside the monorepo
make index
```

Never edit `v1/index.json` by hand. CI fails when it doesn't match the component files.