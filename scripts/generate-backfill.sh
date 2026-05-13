#!/usr/bin/env bash
# scripts/generate-backfill.sh — interactive generator for a GitHub Actions
# → clarity backfill script.
#
# Models the user's choices the way GitHub Actions itself does: per stage,
# pick the WORKFLOW the stage lives in, then pick the SET of jobs that an
# equivalent `clarity-ci-completed` step would `needs:`. The generator emits
# a tailored backfill script to stdout (prompts go to stderr):
#
#     ./scripts/generate-backfill.sh > backfill.sh
#     chmod +x backfill.sh
#     ./backfill.sh --dry-run | less     # review
#     ./backfill.sh                      # execute
#
# Generated script semantics, per (stage, run):
#   started:    time = min(started_at)   over the chosen "started" job set
#   completed:  time = max(completed_at) over the chosen "completed" job set
#               status = aggregate of conclusions (any fail-like → failed,
#                        all success → passed, else skipped)
#
# Requires: gh (authenticated), jq, and git-clarity v0.1.2+ on PATH (the
# generated script invokes `git clarity report --batch`, which was added in
# that release).

set -euo pipefail

log()    { echo "$*" >&2; }
prompt() { printf '%s' "$*" >&2; }

REPO="$(gh repo view --json nameWithOwner -q .nameWithOwner)"
log "Repository: $REPO"

prompt "Trunk branch [main]: "
read -r BRANCH
BRANCH="${BRANCH:-main}"

log
log "Discovering workflows and jobs on branch '$BRANCH'..."

workflows_json="$(gh api --paginate "/repos/$REPO/actions/workflows")"
if [ "$(echo "$workflows_json" | jq '.workflows | length')" = "0" ]; then
    log "No workflows found in $REPO."
    exit 1
fi

# Parallel arrays for workflows that actually have runs on this branch.
# JOBS_FOR[i] is a newline-separated list of job names for workflow i, sampled
# from its most recent run on $BRANCH.
declare -a WF_IDS WF_NAMES JOBS_FOR

while IFS=$'\t' read -r wf_id wf_name; do
    sample_run_id="$(gh api "/repos/$REPO/actions/workflows/$wf_id/runs?branch=$BRANCH&per_page=1" \
        --jq '.workflow_runs[0].id // empty')"
    if [ -z "$sample_run_id" ]; then
        log "  - $wf_name: no runs on '$BRANCH', skipping"
        continue
    fi
    jobs="$(gh api "/repos/$REPO/actions/runs/$sample_run_id/jobs" --jq '.jobs[].name')"
    log "  - $wf_name: $(echo "$jobs" | paste -sd, -)"
    WF_IDS+=("$wf_id")
    WF_NAMES+=("$wf_name")
    JOBS_FOR+=("$jobs")
done < <(echo "$workflows_json" | jq -r '.workflows[] | [.id, .name] | @tsv')

wf_count=${#WF_IDS[@]}
if [ "$wf_count" = "0" ]; then
    log "No workflow runs discovered on branch '$BRANCH'. Try a different branch?"
    exit 1
fi

# pick_workflow sets $wf_idx (0-based) to the user's chosen workflow.
wf_idx=0
pick_workflow() {
    local label="$1"
    log
    log "$label"
    if [ "$wf_count" = "1" ]; then
        log "  (only one workflow has runs; auto-selecting ${WF_NAMES[0]})"
        wf_idx=0
        return
    fi
    local i
    for ((i=0; i<wf_count; i++)); do
        log "  $((i+1))) ${WF_NAMES[i]}"
    done
    while true; do
        prompt "Pick [1-$wf_count]: "
        read -r choice
        if [[ "$choice" =~ ^[0-9]+$ ]] && [ "$choice" -ge 1 ] && [ "$choice" -le "$wf_count" ]; then
            wf_idx=$((choice-1))
            return
        fi
        log "Invalid: $choice"
    done
}

# pick_jobs prompts for a comma-separated subset of jobs in the scoped
# workflow. Sets $picked_jobs to a bash array of job names. An empty input
# falls back to $default_csv (e.g. "1,2,3" or empty for "all").
picked_jobs=()
pick_jobs() {
    local label="$1"
    local default_csv="$2"
    local wf_name="${WF_NAMES[$wf_idx]}"
    local job_lines="${JOBS_FOR[$wf_idx]}"

    # Job names into a local array (newline-separated).
    local jobs=()
    while IFS= read -r line; do
        [ -z "$line" ] && continue
        jobs+=("$line")
    done <<<"$job_lines"
    local n=${#jobs[@]}

    log
    log "$label"
    log "  (in workflow: $wf_name)"
    local i
    for ((i=0; i<n; i++)); do
        log "  $((i+1))) ${jobs[i]}"
    done

    # If there's only one job, skip the prompt.
    if [ "$n" = "1" ]; then
        log "  (only one job; auto-selecting ${jobs[0]})"
        picked_jobs=("${jobs[0]}")
        return
    fi

    # Build the displayed default hint: "all" if default_csv is empty,
    # otherwise the actual indices.
    local default_hint="all"
    [ -n "$default_csv" ] && default_hint="$default_csv"

    while true; do
        prompt "Pick (comma-separated) [default: $default_hint]: "
        read -r choice
        if [ -z "$choice" ]; then
            if [ -z "$default_csv" ]; then
                # Default = all jobs
                picked_jobs=("${jobs[@]}")
                return
            fi
            choice="$default_csv"
        fi
        local parts valid=1
        IFS=',' read -ra parts <<<"$choice"
        picked_jobs=()
        local p
        for p in "${parts[@]}"; do
            p="${p// /}"
            if [[ "$p" =~ ^[0-9]+$ ]] && [ "$p" -ge 1 ] && [ "$p" -le "$n" ]; then
                picked_jobs+=("${jobs[$((p-1))]}")
            else
                valid=0
                break
            fi
        done
        if [ "$valid" = "1" ] && [ ${#picked_jobs[@]} -gt 0 ]; then
            return
        fi
        log "Invalid: $choice"
    done
}

# Returns the comma-separated 1-based indices of the given job names within
# the currently-scoped workflow. Used to surface "same as above" defaults.
indices_csv_for_jobs() {
    local wf_jobs_lines="${JOBS_FOR[$wf_idx]}"
    local -a wf_jobs=()
    while IFS= read -r line; do
        [ -z "$line" ] && continue
        wf_jobs+=("$line")
    done <<<"$wf_jobs_lines"
    local out=""
    local needle haystack i
    for needle in "$@"; do
        for ((i=0; i<${#wf_jobs[@]}; i++)); do
            if [ "${wf_jobs[i]}" = "$needle" ]; then
                [ -n "$out" ] && out+=","
                out+="$((i+1))"
                break
            fi
        done
    done
    echo "$out"
}

# Encode a bash array as a JSON array of strings, for `--argjson` into the
# generated script's jq filters.
json_array() {
    if [ "$#" = "0" ]; then echo '[]'; return; fi
    printf '%s\n' "$@" | jq -R . | jq -s -c .
}

# --- CI ---------------------------------------------------------------------

pick_workflow "Step 1/2: which workflow contains your CI?"
CI_WF_IDX=$wf_idx
CI_WF_ID="${WF_IDS[$CI_WF_IDX]}"
CI_WF_NAME="${WF_NAMES[$CI_WF_IDX]}"

pick_jobs "Jobs that a 'clarity ci started' step would need:" ""
CI_START_JOBS=("${picked_jobs[@]}")

start_csv="$(indices_csv_for_jobs "${CI_START_JOBS[@]}")"
pick_jobs "Jobs that a 'clarity ci completed' step would need:" "$start_csv"
CI_END_JOBS=("${picked_jobs[@]}")

# --- deploy (optional) -------------------------------------------------------

log
prompt "Step 2/2: backfill deploy events too? [y/N]: "
read -r ans
INCLUDE_DEPLOY=0
DEPLOY_WF_ID=""
DEPLOY_WF_NAME=""
DEPLOY_START_JOBS=()
DEPLOY_END_JOBS=()
if [[ "$ans" =~ ^[Yy] ]]; then
    INCLUDE_DEPLOY=1
    pick_workflow "Which workflow contains your deploy?"
    DEPLOY_WF_ID="${WF_IDS[$wf_idx]}"
    DEPLOY_WF_NAME="${WF_NAMES[$wf_idx]}"

    pick_jobs "Jobs that a 'clarity deploy started' step would need:" ""
    DEPLOY_START_JOBS=("${picked_jobs[@]}")

    dep_start_csv="$(indices_csv_for_jobs "${DEPLOY_START_JOBS[@]}")"
    pick_jobs "Jobs that a 'clarity deploy completed' step would need:" "$dep_start_csv"
    DEPLOY_END_JOBS=("${picked_jobs[@]}")
fi

log
log "Writing backfill script to stdout."

# --- emit generated script ---------------------------------------------------

ci_start_json="$(json_array "${CI_START_JOBS[@]}")"
ci_end_json="$(json_array "${CI_END_JOBS[@]}")"

cat <<HEADER
#!/usr/bin/env bash
# Generated by scripts/generate-backfill.sh
# Repo:     $REPO
# Branch:   $BRANCH
# Stages:
#   ci         workflow: $CI_WF_NAME
#              started 'needs': ${CI_START_JOBS[*]}
#              completed 'needs': ${CI_END_JOBS[*]}
HEADER

if [ "$INCLUDE_DEPLOY" = "1" ]; then
    cat <<HEADER
#   deploy     workflow: $DEPLOY_WF_NAME
#              started 'needs': ${DEPLOY_START_JOBS[*]}
#              completed 'needs': ${DEPLOY_END_JOBS[*]}
HEADER
fi

# The BODY heredoc is quoted so $REPO, $BRANCH, the function $1-$4 refs etc.
# appear literally in the generated script. The generator's vars are inlined
# via the unquoted CONFIG heredoc below.
cat <<'BODY'
#
# Walks every run of the chosen workflow(s) on $BRANCH, fetches the jobs once
# per run, aggregates timestamps + conclusions across the selected job set,
# and emits one JSON-Lines event per (commit, stage, edge). The whole stream
# is piped to `git clarity report --batch` so the entire backfill lands as a
# single fetch + commit + push — amortising the per-event round-trip that
# would otherwise dominate runtime.
#
# Pass --dry-run to print the JSONL stream instead of pushing it.
#
# Requires: gh (authenticated), jq, git-clarity on PATH.

set -euo pipefail

DRY_RUN=0
[ "${1:-}" = "--dry-run" ] && DRY_RUN=1

emit_event() {
    # Emits one JSONL line to stdout. The outer wrapper in `main` either pipes
    # this stream to `git clarity report --batch` or prints it (dry-run).
    jq -nc --arg sha "$1" --arg at "$2" --arg stage "$3" --arg status "$4" \
        '{sha: $sha, at: $at, stage: $stage, status: $status}'
}

# Aggregate per-job GitHub conclusions into one clarity status. Any fail-like
# conclusion (failure, timed_out, cancelled) wins; otherwise success across
# the board → passed; else skipped. Empty string means "skip emitting".
aggregate_status() {
    local conclusions="$1"
    local any_fail=0 any_success=0 any_skip=0 any_other=0
    local IFS=,
    local c
    for c in $conclusions; do
        case "$c" in
            failure|timed_out|cancelled) any_fail=1 ;;
            success)                     any_success=1 ;;
            skipped)                     any_skip=1 ;;
            *)                           any_other=1 ;;
        esac
    done
    if [ "$any_fail" = "1" ]; then echo failed
    elif [ "$any_success" = "1" ]; then echo passed
    elif [ "$any_skip" = "1" ]; then echo skipped
    else echo ""
    fi
}

# $1 stage  $2 workflow_id
# $3 JSON array of job names for the "started" event
# $4 JSON array of job names for the "completed" event
backfill_stage() {
    local stage="$1" wf_id="$2" start_set="$3" end_set="$4"
    gh api --paginate "/repos/$REPO/actions/workflows/$wf_id/runs?branch=$BRANCH" \
        --jq '.workflow_runs[].id' \
    | while read -r run_id; do
        jobs_json="$(gh api "/repos/$REPO/actions/runs/$run_id/jobs")"

        start_match="$(echo "$jobs_json" | jq -c --argjson set "$start_set" \
            '[.jobs[] | select(.name as $n | $set | index($n))]')"
        if [ "$(echo "$start_match" | jq 'length')" != "0" ]; then
            sha="$(echo "$start_match" | jq -r '.[0].head_sha')"
            at="$(echo "$start_match" | jq -r 'map(.started_at) | min')"
            emit_event "$sha" "$at" "$stage" started
        fi

        end_match="$(echo "$jobs_json" | jq -c --argjson set "$end_set" \
            '[.jobs[] | select(.name as $n | $set | index($n))]')"
        if [ "$(echo "$end_match" | jq 'length')" != "0" ]; then
            sha="$(echo "$end_match" | jq -r '.[0].head_sha')"
            at="$(echo "$end_match" | jq -r 'map(.completed_at) | max')"
            conclusions="$(echo "$end_match" | jq -r '[.[].conclusion // "null"] | join(",")')"
            status="$(aggregate_status "$conclusions")"
            if [ -n "$status" ]; then
                emit_event "$sha" "$at" "$stage" "$status"
            fi
        fi
    done
}

BODY

cat <<CONFIG
REPO="$REPO"
BRANCH="$BRANCH"

emit_all() {
    backfill_stage ci $CI_WF_ID '$ci_start_json' '$ci_end_json'
CONFIG

if [ "$INCLUDE_DEPLOY" = "1" ]; then
    dep_start_json="$(json_array "${DEPLOY_START_JOBS[@]}")"
    dep_end_json="$(json_array "${DEPLOY_END_JOBS[@]}")"
    cat <<CONFIG
    backfill_stage deploy $DEPLOY_WF_ID '$dep_start_json' '$dep_end_json'
CONFIG
fi

cat <<'TAIL'
}

if [ "$DRY_RUN" = "1" ]; then
    emit_all
else
    emit_all | git clarity report --batch
fi
TAIL
