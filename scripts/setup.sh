#!/bin/bash

set -e # Exit on error

# Detect download tool
if command -v curl >/dev/null 2>&1; then
	DOWNLOAD_TOOL="curl"
	DOWNLOAD_CMD="curl -fsSL -o"
elif command -v wget >/dev/null 2>&1; then
	DOWNLOAD_TOOL="wget"
	DOWNLOAD_CMD="wget -qO"
else
	read -p "Neither curl nor wget is installed, install curl? (y/n): " INSTALL
	if [ "$INSTALL" == "y" ]; then
		install_pkg "curl"
	else
		echo "Error: Neither curl nor wget is installed. Please install one of them and try again."
		exit 1
	fi
fi

echo "Using ${DOWNLOAD_TOOL} for downloads"

# Environment variables with defaults
REPO="yusing/godoxy"
BRANCH=${BRANCH:-"main"}
REPO_URL="https://github.com/$REPO"
# WIKI_URL="${REPO_URL}/wiki"
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

install_pkg() {
	# detect package manager
	if command -v apt >/dev/null 2>&1; then
		apt install -y "$1"
	elif command -v yum >/dev/null 2>&1; then
		yum install -y "$1"
	elif command -v pacman >/dev/null 2>&1; then
		pacman -S --noconfirm "$1"
	else
		echo "Error: No supported package manager found"
		exit 1
	fi
}

check_pkg() {
	local cmd="$1"
	local pkg="$2"
	if ! command -v "$cmd" >/dev/null 2>&1; then
		# check if user is root
		if [ "$EUID" -ne 0 ]; then
			echo "Error: $pkg is not installed and you are not running as root. Please install it and try again."
			exit 1
		fi
		read -p "$pkg is not installed, install it? (y/n): " INSTALL
		if [ "$INSTALL" == "y" ]; then
			install_pkg "$pkg"
		else
			echo "Error: $pkg is not installed. Please install it and try again."
			exit 1
		fi
	fi
}

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

	if [ "$SCRIPT_DEBUG" = "1" ]; then
		cp "$remote_file" "$out_file"
		return
	fi

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

set_env_var() {
	local key="$1"
	local value="$2"
	# uncomment line if it is commented
	sed -i "/^# *${key}=/s/^# *//" "$DOT_ENV_PATH"
	sed -i "s|${key}=.*|${key}=\"${value}\"|" "$DOT_ENV_PATH"
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
			echo "$TIMEZONE"
		fi
	else
		echo "Warning: could not detect timezone, you may set it manually in ${DOT_ENV_PATH} to have correct time in logs"
	fi
}

check_pkg "openssl" "openssl"
check_pkg "docker" "docker-ce"

# Setup required configurations
# 1. Config base directory
mkdir_if_not_exists "$CONFIG_BASE_PATH"

# 2. .env file
fetch_file "$DOT_ENV_EXAMPLE_PATH" "$DOT_ENV_PATH"

# set timezone
get_timezone
if [ -n "$TIMEZONE" ]; then
	set_env_var "TZ" "$TIMEZONE"
fi

# 3. docker-compose.yml
fetch_file "$COMPOSE_EXAMPLE_FILE_NAME" "$COMPOSE_FILE_NAME"

# 4. config.yml
fetch_file "$CONFIG_EXAMPLE_FILE_NAME" "$CONFIG_FILE_PATH"

ask_while_empty "Enter base domain (e.g. domain.com): " BASE_DOMAIN

# 5. setup authentication

# ask for authentication method
ask_multiple_choice AUTH_METHOD "Select authentication method:" \
	"Username/Password" \
	"OIDC" \
	"None"

if [ "$AUTH_METHOD" == "Username/Password" ]; then
	# set random JWT secret
	echo "Setting up JWT secret"
	JWT_SECRET=$(openssl rand -base64 32)
	set_env_var "GODOXY_API_JWT_SECRET" "$JWT_SECRET"

	# ask for user and password
	echo "Setting up login user"
	ask_while_empty "Enter login username: " LOGIN_USER
	ask_while_empty "Enter login password: " LOGIN_PASSWORD
	echo "Setting up login user \"$LOGIN_USER\" with password \"$LOGIN_PASSWORD\""
	set_env_var "GODOXY_API_USER" "$LOGIN_USER"
	set_env_var "GODOXY_API_PASSWORD" "$LOGIN_PASSWORD"
elif [ "$AUTH_METHOD" == "OIDC" ]; then
	# ask for OIDC base domain
	set_env_var "GODOXY_OIDC_REDIRECT_URL" "https://godoxy.${BASE_DOMAIN}/api/auth/callback"

	# ask for OIDC issuer url
	ask_while_empty "Enter OIDC issuer url (e.g. https://pocket-id.domain.com): " OIDC_ISSUER_URL
	set_env_var "GODOXY_OIDC_ISSUER_URL" "$OIDC_ISSUER_URL"

	# ask for OIDC client id
	ask_while_empty "Enter OIDC client id: " OIDC_CLIENT_ID
	set_env_var "GODOXY_OIDC_CLIENT_ID" "$OIDC_CLIENT_ID"

	# ask for OIDC client secret
	ask_while_empty "Enter OIDC client secret: " OIDC_CLIENT_SECRET
	set_env_var "GODOXY_OIDC_CLIENT_SECRET" "$OIDC_CLIENT_SECRET"

	# ask for allowed users
	ask_while_empty "Enter allowed users (comma-separated): " OIDC_ALLOWED_USERS
	set_env_var "GODOXY_OIDC_ALLOWED_USERS" "$OIDC_ALLOWED_USERS"

	# ask for allowed groups
	read -p "Enter allowed groups (comma-separated, leave empty for all): " OIDC_ALLOWED_GROUPS
	[ -n "$OIDC_ALLOWED_GROUPS" ] && set_env_var "GODOXY_OIDC_ALLOWED_GROUPS" "$OIDC_ALLOWED_GROUPS"
else
	echo "Skipping authentication setup"
fi

# 6. setup autocert

# ask if want to enable autocert
echo "Setting up autocert for SSL certificate"
ask_while_empty "Do you want to enable autocert? (y/n): " ENABLE_AUTOCERT

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
		echo "Please submit an issue on ${REPO_URL}/issues for adding your DNS provider"
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

echo "Setup finished"
echo "Starting GoDoxy"
if [ "$SCRIPT_DEBUG" != "1" ]; then
	if docker compose up -d; then
		docker compose logs -f
	else
		echo "Error: Failed to start GoDoxy"
		exit 1
	fi
fi
