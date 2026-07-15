#!/usr/bin/env sh
set -eu

# Placeholder post-create hook exercised by treehouse's integration tests.
# Real adopting repositories use this hook point to seed databases, warm
# caches, or run any other one-time setup after an instance's stack is up.
echo "treehouse: fixture post-create hook ran"
