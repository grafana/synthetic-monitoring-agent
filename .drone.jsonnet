local step(name, commands, image='golang:1.13.10') = {
  name: name,
  commands: commands,
  image: image,
};

local pipeline(name, steps=[]) = {
  kind: 'pipeline',
  type: 'docker',
  name: name,
  steps: [step('runner identification', ['echo $DRONE_RUNNER_NAME'], 'alpine')] + steps,
};

local masterOnly = {
  when: {branch:['master']},
};

local prOnly = {
  when: {event: ['pull_request']},
};

local repo = 'grafana/synthetic-monitoring-agent';

local vault_secret(name, vault_path, key) = {
  kind: 'secret',
  name: name,
  get: {
    path: vault_path,
    name: key,
  },
};

[
  pipeline('build', [
    step('lint', ['make lint']),
    step('test', ['make test']),
    step('build', [
      'git fetch origin --tags',
      './scripts/version',
      './scripts/version > .tags', // save version in special file for docker plugin
      'make build',
    ]),
    // We can't use 'make docker' without making this repo priveleged in drone
    // so we will use the native docker plugin instead for security.
    step('docker build',[],'plugins/docker')+{
      settings:{
        repo: repo,
        dry_run: 'true',
      }
    },
    step('docker push',[],'plugins/docker')
    + {
        settings:{
          repo: repo,
          username: {from_secret: 'docker_username'},
          password: {from_secret: 'docker_password'},
        }
    } + masterOnly,
    step('package', ['make package']) + prOnly,
    step('publish packages', [
      'export GCS_KEY_DIR=$(pwd)/keys',
      'mkdir -p $GCS_KEY_DIR',
      'echo "$GCS_KEY" > $GCS_KEY_DIR/gcs-key.json',
      'make publish-packages',
      ])
      + {environment: {
          GCS_KEY:{from_secret: 'gcs_key'},
          GPG_PRIV_KEY:{from_secret: 'gpg_priv_key'},
        }}
      + masterOnly,
  ]),

  vault_secret('docker_username','infra/data/ci/docker_hub', 'username'),
  vault_secret('docker_password','infra/data/ci/docker_hub', 'password'),
  vault_secret('gcs_key','infra/data/ci/gcp/synthetic-mon-publish-pkgs', 'key'),
  vault_secret('gpg_priv_key','infra/data/ci/gcp/synthetic-mon-publish-pkgs', 'key'),
]