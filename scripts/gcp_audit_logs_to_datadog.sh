#!/usr/bin/env bash

# Iterates over all projects owned by the coder.com organization on GCP and
# ensures that audit log forwarding to datadog is enabled.
#
# Usage: DATADOG_API_KEY=<> ./gcp_audit_logs_to_datadog.sh [--verify] [--yes] [project_ids...]
#
# If no project IDs are specified, all projects owned by the coder.com
# organization will be processed.
#
# If --verify is specified, the script will exit with a non-zero status code if
# any project does not have audit log forwarding setup correctly.

set -euo pipefail
# shellcheck source=scripts/lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

ORG_NAME="coder.com"
PUBSUB_TOPIC_ID="audit-logs-to-datadog"
PUBSUB_SUBSCRIPTION_ID="audit-logs-to-datadog-sub"
LOG_ROUTING_SINK_NAME="audit-logs-to-datadog"

verify=0
yes=0

args="$(getopt -o "" -l verify,yes -- "$@")"
eval set -- "$args"
while true; do
	case "$1" in
	--verify)
		verify=1
		shift
		;;
	--yes)
		yes=1
		shift
		;;
	--)
		shift
		break
		;;
	*)
		error "Unrecognized option: $1"
		;;
	esac
done

if [[ -z "${DATADOG_API_KEY:-}" ]]; then
	error "DATADOG_API_KEY must be set"
fi

# Load projects list.
projects=("$@")
if [[ "${#projects[@]}" == 0 ]]; then
	log "Finding organization ID for $ORG_NAME"
	org_id="$(gcloud organizations list --filter="displayName:$ORG_NAME" --format="value(name)")"
	if [[ -z "$org_id" || "$org_id" =~ [^0-9] ]]; then
		error "Could not find organization with name $ORG_NAME"
	fi

	log "Finding projects in organization $ORG_NAME ($org_id)"
	projects=($(gcloud projects list --filter="parent.id:$org_id" --format="value(projectId)"))
	if [[ "${#projects[@]}" == 0 ]]; then
		error "No projects found in organization $ORG_NAME ($org_id)"
	fi
fi

# Sort the projects list.
readarray -t projects < <(for project in "${projects[@]}"; do echo "$project"; done | sort)

# Print projects list.
log
log "Projects:"
for project in "${projects[@]}"; do
	log "- $project"
done
log
log "Found ${#projects[@]} projects"
log

# Confirm with the user.
if [[ "$yes" == 0 ]]; then
	read -rp "Looks good? [y/N] " read_result
	if [[ "$read_result" != "y" ]]; then
		error "Aborting"
	fi
fi

fail_verify() {
	if [[ "$verify" == 1 ]]; then
		error "$*"
	fi
}

setup_project() {
	project="$1"
	full_pubsub_topic_id="projects/$project/topics/$PUBSUB_TOPIC_ID"
	full_pubsub_sub_id="projects/$project/subscriptions/$PUBSUB_SUBSCRIPTION_ID"

	log "Finding existing pubsub topic in project $project"
	existing_pubsub_topic_id="$(gcloud pubsub topics list --project="$project" --filter="name:$full_pubsub_topic_id" --format="value(name)")"
	if [[ ! -n "$existing_pubsub_topic_id" ]]; then
		log "Found existing pubsub topic $existing_pubsub_topic_id"
	else
		log "Existing topic not found, creating pubsub topic $full_pubsub_topic_id in project $project"
		fail_verify "Attempted to create a pubsub topic in project $project"
		# TODO: figure this part out
		#gcloud pubsub topics create "$full_pubsub_topic_id" --project="$project" 1>&2
	fi

	log "Finding existing pubsub subscription in project $project"
	existing_pubsub_subscription_id="$(gcloud pubsub subscriptions list --project="$project" --filter="name:$full_pubsub_sub_id" --format="value(name)")"
	if [[ ! -n "$existing_pubsub_subscription_id" ]]; then
		log "Found existing pubsub subscription $existing_pubsub_subscription_id"
	else
		log "Existing subscription not found, creating pubsub subscription $PUBSUB_SUBSCRIPTION_ID in project $project"
		fail_verify "Attempted to create a pubsub subscription in project $project"
		# TODO: figure this part out
		#gcloud pubsub subscriptions create "$PUBSUB_SUBSCRIPTION_ID" --topic="$full_pubsub_topic_id" --project="$project" 1>&2
	fi

	log "Finding existing log routing sink in project $project"
	existing_log_routing_sink_name="$(gcloud logging sinks list --project="$project" --filter="name:$LOG_ROUTING_SINK_NAME" --format="value(name)")"
	if [[ ! -n "$existing_log_routing_sink_name" ]]; then
		log "Found existing log routing sink $existing_log_routing_sink_name"
	else
		log "Existing log routing sink not found, creating log routing sink $LOG_ROUTING_SINK_NAME in project $project"
		fail_verify "Attempted to create a log routing sink in project $project"
		# TODO: figure this part out
		#gcloud logging sinks create "$LOG_ROUTING_SINK_NAME" pubsub.googleapis.com/projects/"$project"/topics/"$PUBSUB_TOPIC_ID" --project="$project" --log-filter='protoPayload.serviceName="compute.googleapis.com" OR protoPayload.serviceName="cloudresourcemanager.googleapis.com" OR protoPayload.serviceName="cloudsql.googleapis.com" OR protoPayload.serviceName="container.googleapis.com" OR protoPayload.serviceName="dataproc.googleapis.com" OR protoPayload.serviceName="iam.googleapis.com" OR protoPayload.serviceName="storage.googleapis.com"' 1>&2
	fi

	return 0
}

failed_projects=()

for project in "${projects[@]}"; do
	log
	log "Setting up project $project"
	rc=0
	setup_project "$project" || rc=$?
	if [[ "$rc" != 0 ]]; then
		log
		log "Failed to setup project $project"
		failed_projects+=("$project")
	fi
done

if [[ "${#failed_projects[@]}" != 0 ]]; then
	log
	log "Failed to setup/verify projects:"
	for project in "${failed_projects[@]}"; do
		log "- $project"
	done
	log
	error "Failed to setup/verify ${#failed_projects[@]} projects"
fi
