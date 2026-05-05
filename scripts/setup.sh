#!/bin/sh
# POSIX sh — portable across Linux (dash/busybox), *BSD, macOS.
# Functions use `local` (supported by those shells; not in POSIX yet).

set -e

# Apply sed script(s) to file without relying on sed -i (GNU/BSD differ).
sed_edit_in_place() {
	local _file _tmp
	_file=$1
	shift
	_tmp=$(mktemp "${TMPDIR:-/tmp}/godoxy-setup.XXXXXX") || exit 1
	sed "$@" "$_file" > "$_tmp" || {
		rm -f "$_tmp"
		exit 1
	}
	mv "$_tmp" "$_file" || {
		rm -f "$_tmp"
		exit 1
	}
}

check_cmd() {
	local _missing _cmd
	_missing=
	for _cmd do
		if ! command -v "$_cmd" >/dev/null 2>&1; then
			if [ -z "$_missing" ]; then
				_missing=$_cmd
			else
				_missing="$_missing $_cmd"
			fi
		fi
	done
	if [ -n "$_missing" ]; then
		echo "Error: $_missing unavailable, please install it first"
		exit 1
	fi
}

check_cmd openssl docker

# quit if running user is root
if [ "$(id -u)" -eq 0 ]; then
	echo "Error: Please do not run this script as root"
	exit 1
fi

# check if user has docker permission
if ! docker ps >/dev/null 2>&1; then
	echo "Error: User $(id -un) does not have permission to run docker, please add it to docker group"
	exit 1
fi

# Detect download tool
if command -v curl >/dev/null 2>&1; then
	DOWNLOAD_TOOL="curl"
	DOWNLOAD_CMD="curl -fsSL -o"
elif command -v wget >/dev/null 2>&1; then
	DOWNLOAD_TOOL="wget"
	DOWNLOAD_CMD="wget -qO"
else
	echo "Error: Neither curl nor wget is installed. Please install one of them and try again."
	exit 1
fi

echo "Using ${DOWNLOAD_TOOL} for downloads"

# Environment variables with defaults
REPO="yusing/godoxy"
BRANCH=${BRANCH:-main}
REPO_URL="https://github.com/$REPO"
BASE_URL="${REPO_URL}/raw/${BRANCH}"

# Config paths
CONFIG_BASE_PATH="config"
DOT_ENV_PATH=".env"
DOT_ENV_EXAMPLE_PATH=".env.example"
COMPOSE_FILE_NAME="compose.yml"
COMPOSE_EXAMPLE_FILE_NAME="compose.example.yml"
CONFIG_FILE_NAME="config.yml"
CONFIG_EXAMPLE_FILE_NAME="config.example.yml"
CONFIG_FILE_PATH="${CONFIG_BASE_PATH}/${CONFIG_FILE_NAME}"

echo "Setting up GoDoxy"
echo "Branch: ${BRANCH}"

# Function to check if file/directory exists
has_file_or_dir() {
	[ -e "$1" ]
}

# Function to create directory
mkdir_if_not_exists() {
	if [ ! -d "$1" ]; then
		echo "Creating directory \"$1\""
		mkdir -p "$1"
	fi
}

# Function to create empty file
touch_if_not_exists() {
	if [ ! -f "$1" ]; then
		echo "Creating file \"$1\""
		touch "$1"
	fi
}

# Function to download file
fetch_file() {
	local _rf _of OVERWRITE
	_rf=$1
	_of=$2

	if has_file_or_dir "$_of"; then
		if [ "$_rf" = "$_of" ]; then
			echo "\"$_of\" already exists, not overwriting"
			return
		fi
		printf 'Do you want to overwrite "%s"? (y/n): ' "$_of"
		IFS= read -r OVERWRITE || exit 1
		if [ "$OVERWRITE" != "y" ]; then
			echo "Skipping \"$_rf\""
			return
		fi
	fi

	echo "Downloading \"$_rf\" to \"$_of\""
	if ! $DOWNLOAD_CMD "$_of" "${BASE_URL}/${_rf}"; then
		echo "Error: Failed to download ${_rf}"
		rm -f "$_of"
		exit 1
	fi
	echo "Done"
}

ask_while_empty() {
	local _prompt _var_name _val
	_prompt=$1
	_var_name=$2
	_val=""
	while [ -z "$_val" ]; do
		printf '%s' "$_prompt"
		IFS= read -r _val || exit 1
		if [ -z "$_val" ]; then
			echo "Error: $_var_name cannot be empty, please try again"
		fi
	done
	eval "$_var_name=\$_val"
}

# Usage: ask_multiple_choice VAR_NAME "prompt line" choice1 choice2 ...
pick_nth_arg() {
	local _n _i _arg
	_n=$1
	shift
	_i=1
	for _arg do
		if [ "$_i" -eq "$_n" ]; then
			printf '%s\n' "$_arg"
			return 0
		fi
		_i=$((_i + 1))
	done
	return 1
}

ask_multiple_choice() {
	local _var_name _prompt _valid _i _n_choices _value _selected _choice
	_var_name=$1
	_prompt=$2
	shift 2
	_valid=0
	while [ "$_valid" -eq 0 ]; do
		printf '%s\n' "$_prompt"
		_i=1
		for _choice do
			printf '%s. %s\n' "$_i" "$_choice"
			_i=$((_i + 1))
		done
		_n_choices=$#
		printf 'Enter your choice: '
		IFS= read -r _value || exit 1
		if [ -z "$_value" ]; then
			echo "Error: $_var_name cannot be empty, please try again"
			continue
		fi
		case "$_value" in
			*[!0-9]*)
				echo "Error: invalid choice, please try again"
				continue
				;;
		esac
		if [ "$_value" -lt 1 ] || [ "$_value" -gt "$_n_choices" ]; then
			echo "Error: invalid choice, please try again"
			continue
		fi
		_selected=$(pick_nth_arg "$_value" "$@") || continue
		eval "$_var_name=\$_selected"
		_valid=1
	done
}

get_timezone() {
	if [ -f /etc/timezone ]; then
		TIMEZONE=$(cat /etc/timezone)
		if [ -n "$TIMEZONE" ]; then
			echo "$TIMEZONE"
		fi
	elif command -v timedatectl >/dev/null 2>&1; then
		TIMEZONE=$(timedatectl status | grep "Time zone" | awk '{print $3}')
		if [ -n "$TIMEZONE" ]; then
			echo "Detected timezone: $TIMEZONE"
		fi
	else
		echo "Warning: could not detect timezone, you may set it manually in ${DOT_ENV_PATH} to have correct time in logs"
	fi
}

setenv() {
	local _sk _sv
	_sk=$1
	_sv=$2
	sed_edit_in_place "$DOT_ENV_PATH" "/^# *${_sk}=/s/^# *//"
	sed_edit_in_place "$DOT_ENV_PATH" "s|${_sk}=.*|${_sk}=\"${_sv}\"|"
	echo "${_sk}=${_sv}"
}

# Setup required configurations
# 1. Setup required directories
for dir in config logs error_pages data certs; do
	mkdir_if_not_exists "$dir"
done

# 2. check if rootless docker is used, verify again with user input
if docker info -f "{{println .SecurityOptions}}" | grep rootless >/dev/null 2>&1; then
	ask_while_empty "Rootless docker detected, is this correct? (y/n): " USE_ROOTLESS_DOCKER
	if [ "$USE_ROOTLESS_DOCKER" = "n" ]; then
		USE_ROOTLESS_DOCKER="false"
	else
		USE_ROOTLESS_DOCKER="true"
	fi
fi

# 3. if rootless docker is used, switch to rootless docker compose and .env
if [ "$USE_ROOTLESS_DOCKER" = "true" ]; then
	COMPOSE_EXAMPLE_FILE_NAME="rootless-compose.example.yml"
	DOT_ENV_EXAMPLE_PATH="rootless.env.example"
fi

# 4. .env file
fetch_file "$DOT_ENV_EXAMPLE_PATH" "$DOT_ENV_PATH"

# set random JWT secret
setenv "GODOXY_API_JWT_SECRET" "$(openssl rand -base64 32)"

# set timezone
get_timezone
if [ -n "$TIMEZONE" ]; then
	setenv "TZ" "$TIMEZONE"
fi

# 5. docker-compose.yml
fetch_file "$COMPOSE_EXAMPLE_FILE_NAME" "$COMPOSE_FILE_NAME"

# 6. config.yml
fetch_file "$CONFIG_EXAMPLE_FILE_NAME" "$CONFIG_FILE_PATH"

# 7. setup authentication

# ask for user and password
echo "Setting up login user"
ask_while_empty "Enter login username: " LOGIN_USERNAME
ask_while_empty "Enter login password: " LOGIN_PASSWORD
echo "Setting up login user \"$LOGIN_USERNAME\" with password \"$LOGIN_PASSWORD\""
setenv "GODOXY_API_USER" "$LOGIN_USERNAME"
setenv "GODOXY_API_PASSWORD" "$LOGIN_PASSWORD"

# 8. setup autocert
ask_while_empty "Configure autocert? (y/n): " ENABLE_AUTOCERT

# quit if not using autocert
if [ "$ENABLE_AUTOCERT" = "y" ]; then
	echo "Setting up autocert"
	skip=false

	# ask for domain
	ask_while_empty "Enter domain for autocert: " BASE_DOMAIN

	# ask for email
	ask_while_empty "Enter email for Let's Encrypt: " EMAIL

	# select dns provider
	ask_multiple_choice DNS_PROVIDER "Select DNS provider:" \
		"Cloudflare" \
		"CloudDNS" \
		"DuckDNS" \
		"Other"

	# ask for dns provider credentials
	case "$DNS_PROVIDER" in
	Cloudflare)
		provider="cloudflare"
		printf '%s' "Enter cloudflare zone api key: "
		IFS= read -r auth_token || exit 1
		options_block=$(printf '    auth_token: "%s"' "$auth_token")
		;;
	CloudDNS)
		provider="clouddns"
		printf '%s' "Enter clouddns client_id: "
		IFS= read -r client_id || exit 1
		printf '%s' "Enter clouddns email: "
		IFS= read -r clouddns_email || exit 1
		printf '%s' "Enter clouddns password: "
		IFS= read -r password || exit 1
		options_block=$(printf '    client_id: "%s"\n    email: "%s"\n    password: "%s"' "$client_id" "$clouddns_email" "$password")
		;;
	DuckDNS)
		provider="duckdns"
		printf '%s' "Enter duckdns token: "
		IFS= read -r token || exit 1
		options_block=$(printf '    token: "%s"' "$token")
		;;
	*)
		echo "Please check Wiki for other DNS providers: https://docs.godoxy.dev/DNS-01-Providers"
		echo "Skipping autocert setup"
		skip=true
		options_block=
		;;
	esac

	if [ "$skip" != "true" ]; then
		autocert_config="autocert:
  provider: \"${provider}\"
  email: \"${EMAIL}\"
  domains:
    - \"*.${BASE_DOMAIN}\"
    - \"${BASE_DOMAIN}\"
  options:
${options_block}

"

		_cfg_tmp=$(mktemp "${TMPDIR:-/tmp}/godoxy-setup-cfg.XXXXXX") || exit 1
		printf '%s\n' "$autocert_config" > "$_cfg_tmp"
		cat "$CONFIG_FILE_PATH" >> "$_cfg_tmp"
		mv "$_cfg_tmp" "$CONFIG_FILE_PATH"
	fi
fi

# 9. set uid and gid
if [ "$USE_ROOTLESS_DOCKER" = "true" ]; then
	setenv "DOCKER_SOCKET" "/var/run/user/$(id -u)/docker.sock"
else
	setenv "GODOXY_UID" "$(id -u)"
	setenv "GODOXY_GID" "$(id -g)"
fi

# 10. proxy network (rootless docker only)
if [ "$USE_ROOTLESS_DOCKER" = "true" ]; then
	echo "Setting up proxy network"
	echo "Available networks:"
	docker network ls
	echo
	ask_while_empty "Which network to use for proxy? (default: proxy): " PROXY_NETWORK
	# check if network exists
	if ! docker network ls | grep -q "$PROXY_NETWORK"; then
		ask_while_empty "Network \"$PROXY_NETWORK\" does not exist, do you want to create it? (y/n): " CREATE_NETWORK
		if [ "$CREATE_NETWORK" = "y" ]; then
			docker network create "$PROXY_NETWORK"
			echo "Network \"$PROXY_NETWORK\" created"
		else
			echo "Error: network \"$PROXY_NETWORK\" does not exist, please create it first"
			exit 1
		fi
	fi
	sed_edit_in_place "$COMPOSE_FILE_NAME" "s|proxy: #|\"$PROXY_NETWORK\": #|"
	sed_edit_in_place "$COMPOSE_FILE_NAME" "s|- proxy|- \"$PROXY_NETWORK\"|"
fi

echo "Setup finished"
