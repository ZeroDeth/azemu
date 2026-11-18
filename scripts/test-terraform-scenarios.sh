#!/bin/sh
set -eu

mode=$1
root=$2
major=$3

case "$mode" in
	docker)
		cert="${SSL_CERT_FILE:-$PWD/.azemu/cert-bundle.pem}"
		if [ ! -r "$cert" ]; then
			echo "Missing azemu TLS certificate: $cert. Start azemu first." >&2
			exit 1
		fi
		exec docker run --rm --network host \
			--add-host azemu-ado-kv.vault.localhost:127.0.0.1 \
			--add-host azemu-aks-kv.vault.localhost:127.0.0.1 \
			--add-host azemuotadkv.vault.localhost:127.0.0.1 \
			--add-host azemuotakv.vault.localhost:127.0.0.1 \
			--add-host azemu-redis-kv.vault.localhost:127.0.0.1 \
			--mount "type=bind,src=$PWD/scripts/test-terraform-scenarios.sh,dst=/runner.sh,readonly" \
			--mount "type=bind,src=$PWD/$root,dst=/scenarios,readonly" \
			--mount "type=bind,src=$cert,dst=/azemu-cert.pem,readonly" \
			--user "$(id -u):$(id -g)" -e HOME=/tmp \
			-e SSL_CERT_FILE=/azemu-cert.pem -e NO_PROXY=localhost,127.0.0.1,.vault.localhost \
			--entrypoint /bin/sh hashicorp/terraform:1.15.9 \
			/runner.sh container /scenarios "$major"
		;;
	native|container) ;;
	*) echo "Unknown Terraform scenario runner: $mode" >&2; exit 2 ;;
esac

run_scenario() (
	cd "$1" || exit 1
	if [ "$mode" = container ]; then
		# Keep provider downloads and initialization state out of the host tree.
		TF_DATA_DIR=$(mktemp -d) || exit 1
		export TF_DATA_DIR
		terraform init -input=false -lockfile=readonly && terraform test
	else
		terraform init -input=false && terraform test
	fi
)

failed=""
passed=""
for dir in "$root"/*/; do
	name=$(basename "$dir")
	echo "=== terraform test: $major/$name ==="
	if run_scenario "$dir"; then
		passed="$passed $name"
	else
		failed="$failed $name"
	fi
done
echo "=========== $major scenario summary ==========="
echo "passed:$passed"
echo "failed:$failed"
test -z "$failed"
