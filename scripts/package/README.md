## Packaging the blackbox exporter sidecar

The following scripts are used for packaging and publishing `worldping-blackbox-sidecar`:

Script | Usage
------ | -----
`package.sh` | Creates .deb and .rpm package files
`publish.sh` | Update repos and publish to GCS
`test-install-pkg.sh` | Test installing the sidecar package once published
`gpg-test-vars.sh` | GPG variables to use for testing
`create-repo.sh` | Creates a clean repo with no files. Sync to GCS has been commented out to keep from inadvertently overwriting the published repos.


## Dockerfile.pub

`Dockerfile.pub` at the root of the repo can be used to build and publish the packages by doing the following:

1. Copy a GCP service account `.json` credential file to a directory like `~/keys` named `gcs-key.json` that has Google Storage Object permissions. The directory must be mounted to `/keys` when running the container.
2. `docker build -t wp-publish -f ./Dockerfile.pub .`
3. Set `GPG_PRIV_KEY` to a base64 encoded private key.
4. Set `GPG_KEY_PASSWORD` to a base64 encoded key passphrase.
5. `docker run -it -e GPG_PRIV_KEY -e GPG_KEY_PASSWORD -v ~/keys:/keys -v $(pwd):/worldping --name wp-publish wp-publish sh -c 'make publish-packages'`

\* Add `-e DISABLE_REPO_PUB=1` to run without publishing to the GCS buckets.
