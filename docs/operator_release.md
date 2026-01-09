# Operator Release Process

## Versioning 

The operator uses semantic versioning.
We keep a running changelog in [operator/CHANGELOG.md](../operator/CHANGELOG.md).

## Building a Release

From the operator directory, define the Docker image to build and the release version, then run `release`, e.g.:

```bash
IMG=aistorage/ais-operator:v2.11.0 VERSION=2.11.0 make release
```
This command:
- Generates all required manifests, combined into `operator/dist/ais-operator.yaml`.
- Updates the default image in Kustomize overlays.
- Builds a Helm chart, packages it into [pages/charts](../pages/charts), and updates the Helm repo index for GitHub Pages.
- Automatically creates a corresponding Git commit.

Before merging, ensure the changelog is updated with the latest version for the release.

## Tagging Releases

After merging, tag the commit with the release version and push to the appropriate remotes. 
An example is shown below: 

```bash
git tag v2.11.0
git push origin v2.11.0
git push github v2.11.0
```

## GitHub Release

The GitHub workflow will then trigger the release process, which: 
- Builds and pushes the new operator release image.
- Creates a pre-release draft on GitHub.
- Re-deploys GitHub Pages to publish updated Helm charts.

Once testing and validation are complete, update the GitHub release changelog and mark the release as latest in GitHub.
