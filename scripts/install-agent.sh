#!/bin/sh

set -e

COMMAND="$1"

check_pkg() {
	if ! command -v "$1" >/dev/null 2>&1; then
		echo "$1 could not be found, please install it first"
		exit 1
	fi
}

get_file_mtime() {
	if stat -c %Y "$1" >/dev/null 2>&1; then
		stat -c %Y "$1"
		return
	fi
	if stat -f %m "$1" >/dev/null 2>&1; then
		stat -f %m "$1"
		return
	fi
	echo "Unable to determine file modification time for $1"
	exit 1
}

read_installed_release_timestamp() {
	if [ -f "$version_file" ]; then
		cat "$version_file"
		return
	fi
	if [ -f "$bin_path" ]; then
		get_file_mtime "$bin_path"
		return
	fi
	echo ""
}

write_installed_release_timestamp() {
	mkdir -p "$data_path"
	printf '%s\n' "$bin_last_updated" >"$version_file"
}

detect_init_system() {
	if command -v systemctl >/dev/null 2>&1 && [ -d "/etc/systemd/system" ]; then
		INIT_SYSTEM="systemd"
		echo "System is using systemd"
		return
	fi
	if command -v rc-service >/dev/null 2>&1 && command -v rc-update >/dev/null 2>&1 && [ -d "/etc/init.d" ]; then
		INIT_SYSTEM="openrc"
		echo "System is using OpenRC"
		return
	fi
	echo "Unsupported init system, currently only systemd and OpenRC are supported"
	exit 1
}

service_status_hint() {
	if [ "$INIT_SYSTEM" = "systemd" ]; then
		echo "systemctl status $name"
		return
	fi
	echo "rc-service $name status"
}

service_enable_and_start() {
	if [ "$INIT_SYSTEM" = "systemd" ]; then
		systemctl enable --now "$name"
		return
	fi
	if [ ! -e "/etc/runlevels/default/${name}" ]; then
		rc-update add "$name" default
	fi
	rc-service "$name" start
}

service_is_active() {
	if [ "$INIT_SYSTEM" = "systemd" ]; then
		systemctl is-active "$name" >/dev/null 2>&1
		return
	fi
	rc-service "$name" status >/dev/null 2>&1
}

service_stop() {
	if [ "$INIT_SYSTEM" = "systemd" ]; then
		systemctl stop "$name" || true
		return
	fi
	rc-service "$name" stop || true
}

service_disable() {
	if [ "$INIT_SYSTEM" = "systemd" ]; then
		systemctl disable --now "$name" || true
		systemctl daemon-reload
		return
	fi
	rc-service "$name" stop || true
	rc-update del "$name" default || true
}

service_manager_reload() {
	if [ "$INIT_SYSTEM" = "systemd" ]; then
		systemctl daemon-reload
	fi
}

service_show_status() {
	if [ "$INIT_SYSTEM" = "systemd" ]; then
		systemctl status "$name"
		return
	fi
	rc-service "$name" status || true
}

start_service() {
	service_manager_reload
	# if command is empty
	if [ -z "$COMMAND" ]; then
		echo "Enabling and starting the agent service"
	else
		echo "Reloading the agent service"
	fi
	if ! service_enable_and_start; then
		echo "Failed to enable and start the service. Check with: $(service_status_hint)"
		exit 1
	fi
	echo "Checking if the agent service is started successfully"
	if ! service_is_active; then
		echo "Agent service failed to start, details below:"
		service_show_status
		cat "$log_path"
		exit 1
	fi
	# if command is empty
	if [ -z "$COMMAND" ]; then
		echo "Agent installed successfully"
	else
		echo "Agent updated successfully"
	fi
}

# check if curl and jq are installed
check_pkg curl
check_pkg jq

# check if running user is root
if [ "$EUID" -ne 0 ]; then
	echo "Please run the script as root"
	exit 1
fi

# check variables if command is empty
if [ -z "$COMMAND" ]; then
	if [ -z "$AGENT_NAME" ]; then
		echo "AGENT_NAME is not set"
		exit 1
	fi
	if [ -z "$AGENT_PORT" ]; then
		echo "AGENT_PORT is not set"
		exit 1
	fi
	if [ -z "$AGENT_CA_CERT" ]; then
		echo "AGENT_CA_CERT is not set"
		exit 1
	fi
	if [ -z "$AGENT_SSL_CERT" ]; then
		echo "AGENT_SSL_CERT is not set"
		exit 1
	fi
	if [ -z "$DOCKER_SOCKET" ]; then
		echo "DOCKER_SOCKET is not set"
		exit 1
	fi
	if [ -z "$RUNTIME" ]; then
		echo "RUNTIME is not set"
		exit 1
	fi
fi

# init variables
arch=$(uname -m)
if [ "$arch" = "x86_64" ]; then
	filename="godoxy-agent-linux-amd64"
elif [ "$arch" = "aarch64" ] || [ "$arch" = "arm64" ]; then
	filename="godoxy-agent-linux-arm64"
else
	echo "Unsupported architecture: $arch, expect x86_64 or aarch64/arm64"
	exit 1
fi
repo="yusing/godoxy"
install_path="/usr/local/bin"
name="godoxy-agent"
detect_init_system
bin_path="${install_path}/${name}"
if [ "$INIT_SYSTEM" = "systemd" ]; then
	env_file="/etc/${name}.env"
	service_file="/etc/systemd/system/${name}.service"
else
	env_file="/etc/conf.d/${name}"
	service_file="/etc/init.d/${name}"
fi
log_path="/var/log/godoxy/${name}.log"
log_dir=$(dirname "$log_path")
data_path="/var/lib/${name}"
version_file="${data_path}/release-updated-at"

# check if install path is writable
if [ ! -w "$install_path" ]; then
	echo "Install path is not writable, please check the permissions"
	exit 1
fi

# check if service path is writable
if [ ! -w "$(dirname "$service_file")" ]; then
	echo "Service path is not writable, please check the permissions"
	exit 1
fi

# check if env file is writable
if [ ! -w "$(dirname "$env_file")" ]; then
	echo "Env file is not writable, please check the permissions"
	exit 1
fi

# check if command is uninstall
if [ "$COMMAND" = "uninstall" ]; then
	echo "Uninstalling the agent"
	service_disable
	rm -f "$bin_path"
	rm -f "$env_file"
	rm -f "$service_file"
	rm -rf "$data_path"
	echo "Note: Log file at $log_path is preserved"
	echo "Agent uninstalled successfully"
	exit 0
fi

echo "Finding the latest agent binary"
api_response=$(curl -s -H "Accept: application/vnd.github.v3+json" https://api.github.com/repos/$repo/releases/latest)
if [ -z "$api_response" ]; then
	echo "Failed to get response from GitHub API"
	exit 1
fi

asset=$(echo "$api_response" | jq -r '.assets[] | select(.name | contains("'$filename'"))')
bin_last_updated=$(echo "$asset" | jq -r '.updated_at | fromdateiso8601')
# check if last_updated == mod time of bin_path
installed_release_timestamp=$(read_installed_release_timestamp)
if [ -n "$installed_release_timestamp" ]; then
	if [ "$bin_last_updated" -eq "$installed_release_timestamp" ]; then
		echo "Binary is already up to date, continue? (y/n)"
		IFS= read -r REPLY
		if [ "$REPLY" != "y" ] && [ "$REPLY" != "Y" ]; then
			echo "Aborting"
			exit 0
		fi
	fi
fi

# check if command is update
if [ "$COMMAND" = "update" ]; then
	echo "Stopping the agent"
	service_stop
fi

bin_url=$(echo "$asset" | jq -r '.browser_download_url')
if [ -z "$bin_url" ] || [ "$bin_url" = "null" ]; then
	echo "Failed to find binary for architecture: $arch"
	exit 1
fi

# check if agent is already running
if service_is_active; then
	echo "Agent is already running, stopping it"
	service_stop
	sleep 1
	if service_is_active; then
		echo "Agent is still running, please stop it manually"
		exit 1
	else
		echo "Agent stopped successfully"
	fi
fi

echo "Downloading the agent binary from $bin_url"
if ! curl -L -f "$bin_url" -o $bin_path; then
	echo "Failed to download binary"
	exit 1
fi

echo "Recording installed release timestamp"
write_installed_release_timestamp

echo "Making the agent binary executable"
chmod +x $bin_path

# check if command is update
if [ "$COMMAND" = "update" ]; then
	echo "Starting the agent"
	start_service
	exit 0
fi

echo "Creating the environment file"
cat <<EOF >$env_file
AGENT_NAME="${AGENT_NAME}"
AGENT_PORT="${AGENT_PORT}"
AGENT_CA_CERT="${AGENT_CA_CERT}"
AGENT_SSL_CERT="${AGENT_SSL_CERT}"
DOCKER_SOCKET="${DOCKER_SOCKET}"
RUNTIME="${RUNTIME}"
# use bytedance/sonic library for efficient json handling, disable if you see "SIGILL: illegal instructions"
USE_SONIC_JSON=true
EOF
chmod 600 $env_file

echo "Creating the data directory"
mkdir -p $data_path
chmod 700 $data_path

echo "Creating log directory"
mkdir -p "$log_dir"
touch "$log_path"
chmod 640 "$log_path"

echo "Registering the agent as a service"
if [ "$INIT_SYSTEM" = "systemd" ]; then
	cat <<EOF >$service_file
[Unit]
Description=GoDoxy Agent
After=network.target
After=docker.socket

[Service]
Type=simple
ExecStart=${bin_path}
EnvironmentFile=${env_file}
WorkingDirectory=${data_path}
Restart=always
RestartSec=10
StandardOutput=append:${log_path}
StandardError=append:${log_path}

# Security settings
ProtectSystem=full
ProtectHome=true
NoNewPrivileges=true
ReadWritePaths=${data_path} ${log_path}

# User and group
User=root
Group=root

[Install]
WantedBy=multi-user.target
EOF
else
	cat <<EOF >$service_file
#!/sbin/openrc-run

description="GoDoxy Agent"
command="${bin_path}"
directory="${data_path}"
supervisor=supervise-daemon
output_log="${log_path}"
error_log="${log_path}"
required_files="${env_file}"
respawn_delay=10

depend() {
	need net
	use docker
	after docker
}

start_pre() {
	set -a
	. "${env_file}"
	set +a
}
EOF
fi
if [ "$INIT_SYSTEM" = "systemd" ]; then
	chmod 644 "$service_file"
else
	chmod 755 "$service_file"
fi

start_service
