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
WIKI_URL="${REPO_URL}/wiki"
BASE_URL="${REPO_URL}/raw/${BRANCH}"

# Config paths
CONFIG_BASE_PATH="config"
DOT_ENV_PATH=".env"
DOT_ENV_EXAMPLE_PATH=".env.example"
COMPOSE_FILE_NAME="compose.yml"
COMPOSE_EXAMPLE_FILE_NAME="compose.example.yml"
CONFIG_FILE_NAME="config.yml"
CONFIG_EXAMPLE_FILE_NAME="config.example.yml"
REQUIRED_DIRECTORIES=("config" "logs" "error_pages" "data" "certs")

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

# check if running user is root
if [ "$EUID" -ne 0 ]; then
	echo "Error: Please run this script as root"
	exit 1
fi

check_pkg "openssl" "openssl"
check_pkg "docker" "docker-ce"

# Setup required configurations
# 1. Setup required directories
for dir in "${REQUIRED_DIRECTORIES[@]}"; do
	mkdir_if_not_exists "$dir"
done

# 2. .env file
fetch_file "$DOT_ENV_EXAMPLE_PATH" "$DOT_ENV_PATH"

# set random JWT secret
JWT_SECRET=$(openssl rand -base64 32)
sed -i "s|GODOXY_API_JWT_SECRET=.*|GODOXY_API_JWT_SECRET=${JWT_SECRET}|" "$DOT_ENV_PATH"

# set timezone
get_timezone
if [ -n "$TIMEZONE" ]; then
	sed -i "s|TZ=.*|TZ=${TIMEZONE}|" "$DOT_ENV_PATH"
fi

# 3. docker-compose.yml
fetch_file "$COMPOSE_EXAMPLE_FILE_NAME" "$COMPOSE_FILE_NAME"

# 4. config.yml
fetch_file "$CONFIG_EXAMPLE_FILE_NAME" "${CONFIG_BASE_PATH}/${CONFIG_FILE_NAME}"

# 5. setup authentication

# ask for user and password
echo "Setting up login user"
ask_while_empty "Enter login username: " LOGIN_USERNAME
ask_while_empty "Enter login password: " LOGIN_PASSWORD
echo "Setting up login user \"$LOGIN_USERNAME\" with password \"$LOGIN_PASSWORD\""
sed -i "s|GODOXY_API_USERNAME=.*|GODOXY_API_USERNAME=${LOGIN_USERNAME}|" "$DOT_ENV_PATH"
sed -i "s|GODOXY_API_PASSWORD=.*|GODOXY_API_PASSWORD=${LOGIN_PASSWORD}|" "$DOT_ENV_PATH"

# 6. setup autocert

# ask if want to enable autocert
echo "Setting up autocert for SSL certificate"
ask_while_empty "Do you want to enable autocert? (y/n): " ENABLE_AUTOCERT

# quit if not using autocert
if [ "$ENABLE_AUTOCERT" == "y" ]; then
	# ask for domain
	echo "Setting up autocert"
	ask_while_empty "Enter domain (e.g. example.com): " DOMAIN

	# ask for email
	ask_while_empty "Enter email for Let's Encrypt: " EMAIL

	# ask if using cloudflare
	ask_while_empty "Is cloudflare the current DNS nameserver? (y/n): " USE_CLOUDFLARE

	# ask for cloudflare api key
	if [ "$USE_CLOUDFLARE" = "y" ]; then
		ask_while_empty "Enter cloudflare zone api key: " CLOUDFLARE_API_KEY
		cat <<EOF >>"$CONFIG_BASE_PATH/$CONFIG_FILE_NAME"
autocert:
  provider: cloudflare
  email: $EMAIL
  domains:
    - "*.${DOMAIN}"
    - "${DOMAIN}"
  options:
    auth_token: "$CLOUDFLARE_API_KEY"
EOF
	else
		echo "Not using cloudflare, skipping autocert setup"
		echo "Please refer to ${WIKI_URL}/Supported-DNS-01-Providers for more information"
	fi
fi

# 7. setup permission

echo "Setting up permissions"
chown -R 1000:1000 .
chmod -R 644 .

echo "Setup finished"
