# IMPORTANT: Version values here are duplicated in Taskfile.yml's vars block.
# When bumping the version, update BOTH this file and Taskfile.yml.
# This duplication exists during the Make → go-task transition and will go away when v1 retires.

COLLECTOR_VERSION_MAJOR = 2
COLLECTOR_VERSION_MINOR = 0
COLLECTOR_VERSION_PATCH = 0
COLLECTOR_VERSION = $(COLLECTOR_VERSION_MAJOR).$(COLLECTOR_VERSION_MINOR).$(COLLECTOR_VERSION_PATCH)
COLLECTOR_VERSION_SUFFIX = -SNAPSHOT
COLLECTOR_REVISION = 1
