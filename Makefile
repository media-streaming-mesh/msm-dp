# changelog generation specific
LAST_RELEASE?=$$(git describe --tags $$(git rev-list --tags --max-count=1))
THIS_RELEASE?=$$(git rev-parse --abbrev-ref HEAD)