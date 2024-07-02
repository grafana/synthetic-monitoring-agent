local go_tools_image = 'ghcr.io/grafana/grafana-build-tools:v0.15.0';

local step(name, commands, image=go_tools_image) = {
  name: name,
  commands: commands,
  image: image,
};

local pipeline(name, steps=[]) = {
  kind: 'pipeline',
  type: 'docker',
  name: name,
  image_pull_secrets: [ 'docker_config_json' ],
  steps: [ step('runner identification', [ 'echo $DRONE_RUNNER_NAME' ], 'alpine') ] + steps,
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
  when: { event: [ 'pull_request' ] },
};

local devOnly = {
  when: {
    ref+: [
      'refs/heads/main',
    ],
  },
};

local devAndRelease = {
  when: {
    ref+: [
      'refs/heads/main',
      'refs/tags/v*.*.*',
    ],
  },
};

local docker_repo = 'grafana/synthetic-monitoring-agent';
local gcrio_repo = 'us.gcr.io/kubernetes-dev/synthetic-monitoring-agent';

local docker_auth = {
  username: { from_secret: 'docker_username' },
  password: { from_secret: 'docker_password' },
};

local grcio_auth = {
  config: { from_secret: 'docker_config_json' },
};

local vault_secret(name, vault_path, key) = {
  kind: 'secret',
  name: name,
  get: {
    path: vault_path,
    name: key,
  },
};

local docker_step(tag, os, arch, version='') =
  // We can't use 'make docker' without making this repo priveleged in drone
  // so we will use the native docker plugin instead for security.
  local platform = std.join('/', [ os, arch, if std.length(version) > 0 then version ]);
  step(tag + ' (' + platform + ')', [], 'plugins/docker')
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
  };

local docker_build(os, arch, version='') =
  docker_step('docker build', os, arch, version)
  + dependsOn([ 'build' ]);

local docker_publish(repo, auth, tag, os, arch, version='') =
  docker_step('docker publish to ' + tag, os, arch, version)
  + { settings: { repo: repo, dry_run: 'false' } + auth }
  + dependsOn([ 'test', 'docker build' ]);

[
  pipeline('build', [
    step('deps', [
      'make deps',
      './scripts/enforce-clean',
    ])
    + dependsOn([ 'runner identification' ]),

    step(
      'lint',
      [ 'make lint' ],
      go_tools_image,
    )
    + dependsOn([ 'deps' ]),

    step(
      'build',
      [
        'git fetch origin --tags',
        'git status --porcelain --untracked-files=no',
        'git diff --no-ext-diff --quiet',  // fail if the workspace has modified files
        './scripts/version',
        '{ echo -n latest, ; ./scripts/version ; } > .tags',  // save version in special file for docker plugin
        'make build',
      ],
      go_tools_image,
    )
    + dependsOn([ 'deps' ]),

    step('test', [ 'make test' ])
    + dependsOn([ 'lint', 'build' ]),

    docker_build('linux', 'amd64'),
    docker_build('linux', 'arm64', 'v8'),

    step('docker build', [ 'true' ], 'alpine')
    + dependsOn([
      'docker build (linux/amd64)',
      'docker build (linux/arm64/v8)',
    ]),

    docker_publish(gcrio_repo, grcio_auth, 'gcr.io', 'linux', 'amd64') + devAndRelease,
    // docker_publish(gcrio_repo, grcio_auth, 'gcr.io', 'linux', 'arm', 'v7') + devAndRelease,
    // docker_publish(gcrio_repo, grcio_auth, 'gcr.io', 'linux', 'arm64', 'v8') + devAndRelease,

    docker_publish(docker_repo, docker_auth, 'docker', 'linux', 'amd64') + releaseOnly,
    // docker_publish(docker_repo, docker_auth, 'docker', 'linux', 'arm', 'v7') + releaseOnly,
    // docker_publish(docker_repo, docker_auth, 'docker', 'linux', 'arm64', 'v8') + releaseOnly,

    step('docker publish (dev)', [ 'true' ], 'alpine')
    + dependsOn([
      'docker publish to gcr.io (linux/amd64)',
      // 'docker publish to gcr.io (linux/arm/v7)',
      // 'docker publish to gcr.io (linux/arm64/v8)',
    ])
    + devAndRelease,

    step('docker publish (release)', [ 'true' ], 'alpine')
    + dependsOn([
      'docker publish to gcr.io (linux/amd64)',
      // 'docker publish to gcr.io (linux/arm/v7)',
      // 'docker publish to gcr.io (linux/arm64/v8)',
      'docker publish to docker (linux/amd64)',
      // 'docker publish to docker (linux/arm/v7)',
      // 'docker publish to docker (linux/arm64/v8)',
    ])
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
      image: 'us.gcr.io/kubernetes-dev/drone/plugins/argo-cli',
    }
    + dependsOn([ 'docker publish (dev)' ])
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
      image: 'us.gcr.io/kubernetes-dev/drone/plugins/argo-cli',
    }
    + dependsOn([ 'docker publish (release)' ])
    + releaseOnly,

  ]),

  // Build and release packages
  // Tested in PRs by installing the packages on a systemd container
  pipeline('release') + {
    trigger: {
      event: [ 'tag', 'pull_request' ],
    },
    volumes+: [
      {
        name: 'cgroup',
        host: {
          path: '/sys/fs/cgroup',
        },
      },
      {
        name: 'docker',
        host: {
          path: '/var/run/docker.sock',
        },
      },
    ],
    // Launch systemd containers to test the packages
    services: [
      {
        name: 'systemd-debian',
        image: 'jrei/systemd-debian:12',
        volumes: [
          {
            name: 'cgroup',
            path: '/sys/fs/cgroup',
          },
        ],
        privileged: true,
      },
      {
        name: 'systemd-centos',
        image: 'jrei/systemd-centos:8',
        volumes: [
          {
            name: 'cgroup',
            path: '/sys/fs/cgroup',
          },
        ],
        privileged: true,
      },
    ],
    steps+: [
      step('fetch', [ 'git fetch --tags' ], image='docker:git'),
      step('write-key', [ 'printf "%s" "$NFPM_SIGNING_KEY" > $NFPM_SIGNING_KEY_FILE' ]) + {
        environment: {
          NFPM_SIGNING_KEY: { from_secret: 'gpg_private_key' },
          NFPM_SIGNING_KEY_FILE: '/drone/src/release-private-key.key',
        },
      },
      step('test release', [ 'make release-snapshot' ]) + {
        environment: {
          NFPM_DEFAULT_PASSPHRASE: { from_secret: 'gpg_passphrase' },
          NFPM_SIGNING_KEY_FILE: '/drone/src/release-private-key.key',
        },
      },
      step('test deb package', [ './scripts/package/verify-deb-install.sh' ], image='docker') + {
        volumes: [
          {
            name: 'docker',
            path: '/var/run/docker.sock',
          },
        ],
        privileged: true,
      },
      step('test rpm package', [ './scripts/package/verify-rpm-install.sh' ], image='docker') + {
        volumes: [
          {
            name: 'docker',
            path: '/var/run/docker.sock',
          },
        ],
        privileged: true,
      },
      step('release', [ 'make release' ]) + releaseOnly + {
        environment: {
          GITHUB_TOKEN: { from_secret: 'gh_token' },
          NFPM_DEFAULT_PASSPHRASE: { from_secret: 'gpg_passphrase' },
          NFPM_SIGNING_KEY_FILE: '/drone/src/release-private-key.key',
        },
      },
    ],
  } + {
    trigger: {
      ref: [
        'refs/pull/**',
        'refs/tags/v*.*.*',
      ],  // Only on tags and PRs
      repo: [ 'grafana/*' ],  // Only trigger this pipeline for the Grafana org (no forks)
    },
  },

  vault_secret('docker_username', 'infra/data/ci/docker_hub', 'username'),
  vault_secret('docker_password', 'infra/data/ci/docker_hub', 'password'),
  vault_secret('docker_config_json', 'infra/data/ci/gcr-admin', '.dockerconfigjson'),
  vault_secret('argo_token', 'infra/data/ci/argo-workflows/trigger-service-account', 'token'),

  // secret needed to publish a Github release
  vault_secret('gh_token', 'infra/data/ci/github/grafanabot', 'pat'),

  // secret to sign linux packages
  vault_secret('gpg_passphrase', 'infra/data/ci/packages-publish/gpg', 'passphrase'),
  vault_secret('gpg_private_key', 'infra/data/ci/packages-publish/gpg', 'private-key'),
]
