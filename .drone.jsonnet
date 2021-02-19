local step(name, commands, image='golang:1.16') = {
  name: name,
  commands: commands,
  image: image,
};

local pipeline(name, steps=[]) = {
  kind: 'pipeline',
  type: 'docker',
  name: name,
  steps: [step('runner identification', ['echo $DRONE_RUNNER_NAME'], 'alpine')] + steps,
  trigger+: {
    ref+: [
      'refs/heads/master',
      'refs/pull/**',
      'refs/tags/v*.*.*',
    ],
  },
};

local releaseOnly = {
  when: {
    ref+: [
      'refs/heads/master',
      'refs/tags/v*.*.*',
    ],
  },
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
      'git status --porcelain --untracked-files=no',
      'git diff --no-ext-diff --quiet', // fail if the workspace has modified files
      './scripts/version',
      '{ echo -n latest, ; ./scripts/version ; } > .tags', // save version in special file for docker plugin
      'make build',
    ]),

    // We can't use 'make docker' without making this repo priveleged in drone
    // so we will use the native docker plugin instead for security.
    step('docker build', [], 'plugins/docker')
    + {
      settings:{
        repo: repo,
        dry_run: 'true',
      }
    },

    step('docker push', [], 'plugins/docker')
    + {
        settings:{
          repo: repo,
          username: {from_secret: 'docker_username'},
          password: {from_secret: 'docker_password'},
        }
    }
    + releaseOnly,

    step('package', ['make package'])
    + prOnly,

    step('publish packages', [
      'export GCS_KEY_DIR=$(pwd)/keys',
      'mkdir -p $GCS_KEY_DIR',
      'echo "$GCS_KEY" | base64 -d > $GCS_KEY_DIR/gcs-key.json',
      'make publish-packages',
      ])
    + {
        environment: {
          GCS_KEY:{from_secret: 'gcs_key'},
          GPG_PRIV_KEY:{from_secret: 'gpg_priv_key'},
          PUBLISH_PROD_PKGS: "1",
        }
      }
    + releaseOnly,
  ]),

  vault_secret('docker_username','infra/data/ci/docker_hub', 'username'),
  vault_secret('docker_password','infra/data/ci/docker_hub', 'password'),
  vault_secret('gcs_key','infra/data/ci/gcp/synthetic-mon-publish-pkgs', 'key'),
  vault_secret('gpg_priv_key','infra/data/ci/gcp/synthetic-mon-publish-pkgs', 'gpg_priv_key'),
]
