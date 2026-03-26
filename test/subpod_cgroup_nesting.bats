#!/usr/bin/env bats

load helpers

function setup() {
	setup_test
	if [[ $RUNTIME_TYPE == vm ]]; then
		skip "not applicable to vm runtime type"
	fi
}

function teardown() {
	cleanup_test
}

# Sub-pod nesting: child pod annotated with parent-pod-uid.crio.io gets cgroups under the
# parent pod scope (.../crio-<parentID>.scope/subpods/<childID>/crio-<childID>).
@test "subpod cgroup nesting places child under parent scope" {
	if ! is_cgroup_v2; then
		skip "sub-pod nesting requires cgroup v2"
	fi
	if [[ "$CONTAINER_CGROUP_MANAGER" != "systemd" ]]; then
		skip "sub-pod nesting requires systemd cgroup driver"
	fi

	CONTAINER_CGROUP_MANAGER="systemd" CONTAINER_DROP_INFRA_CTR=false start_crio

	local parent_uid="11111111-1111-1111-1111-111111111111"
	local child_uid="22222222-2222-2222-2222-222222222222"

	jq --arg slice "Burstablecriotest123.slice" \
		--arg uid "$parent_uid" \
		'.linux.cgroup_parent = $slice |
		.metadata.uid = $uid |
		.metadata.name = "bats-subpod-parent"' \
		"$TESTDATA"/sandbox_config.json >"$TESTDIR"/sandbox_parent.json

	local parent_id
	parent_id=$(crictl runp "$TESTDIR"/sandbox_parent.json)

	jq --arg slice "Burstablecriotest123.slice" \
		--arg puid "$parent_uid" \
		--arg cuid "$child_uid" \
		'.linux.cgroup_parent = $slice |
		.metadata.uid = $cuid |
		.metadata.name = "bats-subpod-child" |
		.annotations["parent-pod-uid.crio.io"] = $puid' \
		"$TESTDATA"/sandbox_config.json >"$TESTDIR"/sandbox_child.json

	local child_id
	child_id=$(crictl runp "$TESTDIR"/sandbox_child.json)

	local subpod_base
	subpod_base=$(crictl inspectp "$child_id" | jq -r '.info.runtimeSpec.annotations["io.kubernetes.cri-o.SubpodCgroupBase"] // empty')

	[[ -n "$subpod_base" ]]
	[[ -d "$subpod_base" ]]
	[[ "$subpod_base" == *"/subpods/$child_id"* ]]
	# Nesting base is under the parent pod scope directory (crio-<parentSandboxID>.scope).
	[[ "$subpod_base" == *"crio-$parent_id"* ]]

	[[ "$subpod_base" =~ ^/sys/fs/cgroup/ ]]

	crictl rmp -a -f
}
