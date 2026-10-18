#!/usr/bin/env bash
# Wipes a Hardcover TEST account's library and the local sync state so the
# tool can be re-run from scratch. Audible credentials are kept.
#
# Usage: scripts/reset-test-account.sh <expected-hardcover-username>
#
# The username argument is a guard: the script refuses to delete anything
# unless the token in .env belongs to exactly that user.
set -euo pipefail

cd "$(dirname "$0")/.."

ENDPOINT="https://api.hardcover.app/v1/graphql"
VOLUME="audible-hardcover-sync_sync-data"
REQUEST_GAP=1.1 # seconds; stays under the 60/min rate limit

expected_user="${1:-}"
if [[ -z "$expected_user" ]]; then
	echo "usage: $0 <expected-hardcover-username>" >&2
	exit 2
fi

token=$(grep '^HARDCOVER_API_TOKEN=' .env | cut -d= -f2- | tr -d '"\r')
if [[ -z "$token" ]]; then
	echo "HARDCOVER_API_TOKEN not set in .env" >&2
	exit 1
fi

gql() {
	local body
	body=$(jq -nc --arg q "$1" --argjson v "${2:-{\}}" '{query: $q, variables: $v}')
	curl -sf "$ENDPOINT" -H "content-type: application/json" -H "authorization: $token" -d "$body"
}

me=$(gql '{ me { username user_books { id } } }')
username=$(jq -r '.data.me[0].username' <<<"$me")
if [[ "$username" != "$expected_user" ]]; then
	echo "token belongs to '$username', not '$expected_user' — refusing to delete" >&2
	exit 1
fi

mapfile -t ids < <(jq -r '.data.me[0].user_books[].id' <<<"$me")
echo "deleting ${#ids[@]} library entries from $username"

failed=0
for id in "${ids[@]}"; do
	if ! out=$(gql 'mutation($id: Int!) { delete_user_book(id: $id) { id } }' "{\"id\": $id}") ||
		[[ $(jq '.errors // empty | length' <<<"$out") != "" ]]; then
		echo "  failed: user_book $id: ${out:-request error}" >&2
		failed=$((failed + 1))
	fi
	sleep "$REQUEST_GAP"
done

remaining=$(gql '{ me { user_books_aggregate { aggregate { count } } } }' |
	jq '.data.me[0].user_books_aggregate.aggregate.count')
echo "hardcover: $failed failed, $remaining entries remaining"

echo "resetting local sync state (keeping Audible credentials)"
docker compose stop sync-tool >/dev/null
docker run --rm -v "$VOLUME:/data" alpine:3.20 rm -f /data/state.db
docker compose start sync-tool >/dev/null
echo "done — run: docker compose run --rm sync-tool sync"
