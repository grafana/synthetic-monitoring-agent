local step(name, commands, image='golang:1.17') = {
  name: name,
  commands: commands,
  image: image,
};

local pipeline(name, steps=[]) = {
  kind: 'pipeline',
  type: 'docker',
  name: name,
  image_pull_secrets: ['docker_config_json'],
  steps: [step('runner identification', ['echo $DRONE_RUNNER_NAME'], 'alpine')] + steps,
  trigger+: {
    ref+: [
      'refs/heads/main',
      'refs/pull/**',
      'refs/tags/v*.*.*',
    ],
  },
};

local dependsOn(steps=[]) = {
  depends_on: steps,
};

local releaseOnly = {
  when: {
    ref+: [
      'refs/tags/v*.*.*',
    ],
  },
};

local prOnly = {
  when: { event: ['pull_request'] },
};

local devOnly = {
  when: {
    ref+: [
      'refs/heads/main',
    ],
  },
};


local docker_repo = 'grafana/synthetic-monitoring-agent';
local gcrio_repo = 'us.gcr.io/kubernetes-dev/synthetic-monitoring-agent';

local vault_secret(name, vault_path, key) = {
  kind: 'secret',
  name: name,
  get: {
    path: vault_path,
    name: key,
  },
};

local docker_build(os, arch, version='') =
  // We can't use 'make docker' without making this repo priveleged in drone
  // so we will use the native docker plugin instead for security.
  local platform = std.join('/', [ os, arch, if std.length(version) > 0 then version ]);
  step('docker build (' + platform + ')', [], 'plugins/docker')
  + {
    environment: {
      DOCKER_BUILDKIT: '1',
    },
    settings: {
      repo: docker_repo,
      dry_run: 'true',
      build_args: [
        'TARGETPLATFORM=' + platform,
        'TARGETOS=' + os,
        'TARGETARCH=' + arch,
      ] + if std.length(version) > 0 then [
        'TARGETVARIANT=' + version,
      ] else [],
    },
  }
  + dependsOn(['build']);

[
  pipeline('build', [
    step('deps', [
      'make deps',
      './scripts/enforce-clean',
    ])
    + dependsOn(['runner identification']),

    step('lint', ['make lint'])
    + dependsOn(['deps']),

    step('test', ['make test'])
    + dependsOn(['lint']),

    step('build', [
      'git fetch origin --tags',
      'git status --porcelain --untracked-files=no',
      'git diff --no-ext-diff --quiet',  // fail if the workspace has modified files
      './scripts/version',
      '{ echo -n latest, ; ./scripts/version ; } > .tags',  // save version in special file for docker plugin
      'make build',
    ])
    + dependsOn(['deps']),

    docker_build('linux', 'amd64'),
    docker_build('linux', 'arm', 'v7'),
    docker_build('linux', 'arm64', 'v8'),

    step('docker build', ['true'], 'alpine')
    + dependsOn([
      'docker build (linux/amd64)',
      'docker build (linux/arm/v7)',
      'docker build (linux/arm64/v8)',
    ]),

    step('docker push to docker.com', [], 'plugins/docker')
    + {
      settings: {
        repo: docker_repo,
        username: { from_secret: 'docker_username' },
        password: { from_secret: 'docker_password' },
      },
    }
    + dependsOn(['test', 'docker build'])
    + releaseOnly,

    step('docker push to gcr.io (dev)', [], 'plugins/docker')
    + {
      settings: {
        repo: gcrio_repo,
        config: {from_secret: 'docker_config_json'},
      },
    }
    + dependsOn(['test', 'docker build'])
    + devOnly,

    step('docker push to gcr.io (release)', [], 'plugins/docker')
    + {
      settings: {
        repo: gcrio_repo,
        config: {from_secret: 'docker_config_json'},
      },
    }
    + dependsOn(['test', 'docker build'])
    + releaseOnly,

    step('package', ['make package'])
    + dependsOn(['test', 'docker build'])
    + prOnly,

    step('publish packages', [
      'export GCS_KEY_DIR=$(pwd)/keys',
      'mkdir -p $GCS_KEY_DIR',
      'echo "$GCS_KEY" | base64 -d > $GCS_KEY_DIR/gcs-key.json',
      'make publish-packages',
    ])
    + {
      environment: {
        GCS_KEY: { from_secret: 'gcs_key' },
        GPG_PRIV_KEY: { from_secret: 'gpg_priv_key' },
        PUBLISH_PROD_PKGS: '1',
      },
    }
    + dependsOn(['package'])
    + releaseOnly,

    step('trigger argo workflow (dev)', [])
    + {
        settings: {
          namespace: 'synthetic-monitoring-cd',
          token: { from_secret: 'argo_token' },
          command: std.strReplace(|||
            submit --from workflowtemplate/deploy-synthetic-monitoring-agent
            --name deploy-synthetic-monitoring-agent-$(./scripts/version)
            --parameter mode=dev
            --parameter dockertag=$(./scripts/version)
            --parameter commit=${DRONE_COMMIT}
            --parameter commit_author=${DRONE_COMMIT_AUTHOR}
            --parameter commit_link=${DRONE_COMMIT_LINK}
          |||, '\n', ' '),
          add_ci_labels: true,
        },
        image: 'us.gcr.io/kubernetes-dev/drone/plugins/argo-cli'
    }
    + dependsOn(['docker push to gcr.io (dev)'])
    + devOnly,

    step('trigger argo workflow (release)', [])
    + {
        settings: {
          namespace: 'synthetic-monitoring-cd',
          token: { from_secret: 'argo_token' },
          command: std.strReplace(|||
            submit --from workflowtemplate/deploy-synthetic-monitoring-agent
            --name deploy-synthetic-monitoring-agent-$(./scripts/version)
            --parameter mode=release
            --parameter dockertag=$(./scripts/version)
            --parameter commit=${DRONE_COMMIT}
            --parameter commit_author=${DRONE_COMMIT_AUTHOR}
            --parameter commit_link=${DRONE_COMMIT_LINK}
          |||, '\n', ' '),
          add_ci_labels: true,
        },
        image: 'us.gcr.io/kubernetes-dev/drone/plugins/argo-cli'
   }
   + dependsOn(['docker push to gcr.io (release)'])
   + releaseOnly,

  ]),

  vault_secret('docker_username', 'infra/data/ci/docker_hub', 'username'),
  vault_secret('docker_password', 'infra/data/ci/docker_hub', 'password'),
  vault_secret('gcs_key', 'infra/data/ci/gcp/synthetic-mon-publish-pkgs', 'key'),
  vault_secret('gpg_priv_key', 'infra/data/ci/gcp/synthetic-mon-publish-pkgs', 'gpg_priv_key'),
  vault_secret('docker_config_json', 'infra/data/ci/gcr-admin', '.dockerconfigjson'),
  vault_secret('argo_token', 'infra/data/ci/argo-workflows/trigger-service-account', 'token'),
]
