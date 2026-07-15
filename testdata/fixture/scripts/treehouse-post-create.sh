#!/usr/bin/env sh
set -eu

# Post-create hook exercised by treehouse's integration tests. Real
# adopting repositories use this hook point to seed databases, warm caches,
# or run any other one-time setup after an instance's stack is up — which
# only works if the stack really is up by the time this hook runs. This
# check is the regression guard for that ordering: it fails the hook (and
# therefore `create`, with StateProvisionFailed) if fewer than the fixture's
# two services are running yet.
running=$(docker compose --project-name "$COMPOSE_PROJECT_NAME" ps --status running --format '{{.Name}}' | wc -l | tr -d ' ')
if [ "$running" -lt 2 ]; then
	echo "treehouse: post-create hook ran before the compose stack was up (expected 2 running containers, got $running)" >&2
	exit 1
fi

echo "treehouse: fixture post-create hook ran"
