#!/bin/bash

set -e # Exit on error

check_cmd() {
	not_available=()
	for cmd in "$@"; do
		if ! command -v "$cmd" >/dev/null 2>&1; then
			not_available+=("$cmd")
		fi
	done
	if [ "${#not_available[@]}" -gt 0 ]; then
		echo "Error: ${not_available[*]} unavailable, please install it first"
		exit 1
	fi
}

check_cmd openssl docker

# quit if running user is root
if [ "$EUID" -eq 0 ]; then
	echo "Error: Please do not run this script as root"
	exit 1
fi

# check if user has docker permission
if ! docker ps >/dev/null 2>&1; then
	echo "Error: User $USER does not have permission to run docker, please add it to docker group"
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
BRANCH=${BRANCH:-"main"}
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
REQUIRED_DIRECTORIES=("config" "logs" "error_pages" "data" "certs")

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
	local remote_file="$1"
	local out_file="$2"

	if has_file_or_dir "$out_file"; then
		if [ "$remote_file" = "$out_file" ]; then
			echo "\"$out_file\" already exists, not overwriting"
			return
		fi
		read -p "Do you want to overwrite \"$out_file\"? (y/n): " OVERWRITE
		if [ "$OVERWRITE" != "y" ]; then
			echo "Skipping \"$remote_file\""
			return
		fi
	fi

	echo "Downloading \"$remote_file\" to \"$out_file\""
	if ! $DOWNLOAD_CMD "$out_file" "${BASE_URL}/${remote_file}"; then
		echo "Error: Failed to download ${remote_file}"
		rm -f "$out_file" # Clean up partial download
		exit 1
	fi
	echo "Done"
}

ask_while_empty() {
	local prompt="$1"
	local var_name="$2"
	local value=""
	while [ -z "$value" ]; do
		read -p "$prompt" value
		if [ -z "$value" ]; then
			echo "Error: $var_name cannot be empty, please try again"
		fi
	done
	eval "$var_name=\"$value\""
}

ask_multiple_choice() {
	local var_name="$1"
	local prompt="$2"
	shift 2
	local choices=("$@")
	local n_choices="${#choices[@]}"
	local value=""
	local valid=0
	while [ $valid -eq 0 ]; do
		echo -e "$prompt"
		for i in "${!choices[@]}"; do
			echo "$((i + 1)). ${choices[$i]}"
		done
		read -p "Enter your choice: " value
		if [ -z "$value" ]; then
			echo "Error: $var_name cannot be empty, please try again"
		fi
		if [ "$value" -gt "$n_choices" ] || [ "$value" -lt 1 ]; then
			echo "Error: invalid choice, please try again"
		else
			valid=1
		fi
	done
	eval "$var_name=\"${choices[$((value - 1))]}\""
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
	local key="$1"
	local value="$2"
	# uncomment line if it is commented
	sed -i "/^# *${key}=/s/^# *//" "$DOT_ENV_PATH"
	sed -i "s|${key}=.*|${key}=\"${value}\"|" "$DOT_ENV_PATH"
	echo "${key}=${value}"
}

# Setup required configurations
# 1. Setup required directories
for dir in "${REQUIRED_DIRECTORIES[@]}"; do
	mkdir_if_not_exists "$dir"
done

# 2. .env file
fetch_file "$DOT_ENV_EXAMPLE_PATH" "$DOT_ENV_PATH"

# set random JWT secret
setenv "GODOXY_API_JWT_SECRET" "$(openssl rand -base64 32)"

# set timezone
get_timezone
if [ -n "$TIMEZONE" ]; then
	setenv "TZ" "$TIMEZONE"
fi

# 3. docker-compose.yml
fetch_file "$COMPOSE_EXAMPLE_FILE_NAME" "$COMPOSE_FILE_NAME"

# 4. config.yml
fetch_file "$CONFIG_EXAMPLE_FILE_NAME" "$CONFIG_FILE_PATH"

# 5. setup authentication

# ask for user and password
echo "Setting up login user"
ask_while_empty "Enter login username: " LOGIN_USERNAME
ask_while_empty "Enter login password: " LOGIN_PASSWORD
echo "Setting up login user \"$LOGIN_USERNAME\" with password \"$LOGIN_PASSWORD\""
setenv "GODOXY_API_USER" "$LOGIN_USERNAME"
setenv "GODOXY_API_PASSWORD" "$LOGIN_PASSWORD"

# 6. setup autocert
ask_while_empty "Configure autocert? (y/n): " ENABLE_AUTOCERT

# quit if not using autocert
if [ "$ENABLE_AUTOCERT" == "y" ]; then
	# ask for domain
	echo "Setting up autocert"
	skip=false

	# ask for email
	ask_while_empty "Enter email for Let's Encrypt: " EMAIL

	# select dns provider
	ask_multiple_choice DNS_PROVIDER "Select DNS provider:" \
		"Cloudflare" \
		"CloudDNS" \
		"DuckDNS" \
		"Other"

	# ask for dns provider credentials
	if [ "$DNS_PROVIDER" == "Cloudflare" ]; then
		provider="cloudflare"
		read -p "Enter cloudflare zone api key: " auth_token
		options=("auth_token: \"$auth_token\"")
	elif [ "$DNS_PROVIDER" == "CloudDNS" ]; then
		provider="clouddns"
		read -p "Enter clouddns client_id: " client_id
		read -p "Enter clouddns email: " email
		read -p "Enter clouddns password: " password
		options=(
			"client_id: \"$client_id\""
			"email: \"$email\""
			"password: \"$password\""
		)
	elif [ "$DNS_PROVIDER" == "DuckDNS" ]; then
		provider="duckdns"
		read -p "Enter duckdns token: " token
		options=("token: \"$token\"")
	else
		echo "Please check Wiki for other DNS providers: https://docs.godoxy.dev/DNS-01-Providers"
		echo "Skipping autocert setup"
		skip=true
	fi
	if [ "$skip" == false ]; then
		autocert_config="
autocert:
  provider: \"${provider}\"
  email: \"${EMAIL}\"
  domains:
    - \"*.${BASE_DOMAIN}\"
    - \"${BASE_DOMAIN}\"
  options:
"
		for option in "${options[@]}"; do
			autocert_config+="    ${option}\n"
		done
		autocert_config+="\n"
		echo -e "${autocert_config}$(<"$CONFIG_FILE_PATH")" >"$CONFIG_FILE_PATH"
	fi
fi

# 7. set uid and gid
setenv "GODOXY_UID" "$(id -u)"
setenv "GODOXY_GID" "$(id -g)"

echo "Setup finished"
