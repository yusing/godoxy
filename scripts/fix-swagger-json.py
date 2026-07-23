# Normalize Swagger extensions for the TypeScript generator. Fields are
# non-nullable and required by default; explicit x-omitempty fields remain
# optional in the generated client.
import json

path = "internal/api/v1/docs/swagger.json"

with open(path, "r") as f:
    data = json.load(f)

if "netip.Addr" in data["definitions"] and isinstance(
    data["definitions"]["netip.Addr"], dict
):
    # MarshalText()
    data["definitions"]["netip.Addr"] = {
        "anyOf": [
            {"type": "string", "format": "ipv4"},
            {"type": "string", "format": "ipv6"},
        ],
        "x-nullable": False,
        "x-omitempty": False,
    }


def set_non_nullable(data):
    if not isinstance(data, dict):
        return
    if "x-nullable" not in data:
        # swagger-typescript-api v13 no longer derives optional properties
        # from x-omitempty. It still uses x-nullable for that distinction, so
        # preserve the backend's omission contract in generated clients.
        data["x-nullable"] = bool(data.get("x-omitempty", False))
    if "x-omitempty" not in data and not data["x-nullable"]:
        data["x-omitempty"] = False
    if "type" not in data:
        return
    if data["type"] == "object" and "properties" in data:
        if "required" in data:
            for k, v in data["properties"].items():
                if k in data["required"]:
                    set_non_nullable(v)
        else:
            for v in data["properties"].values():
                set_non_nullable(v)
    if data["type"] == "array":
        for v in data["items"]:
            set_non_nullable(v)


def set_operation_id(data):
    if isinstance(data, dict):
        if "x-id" in data:
            data["operationId"] = data["x-id"]
            return
        for v in data.values():
            set_operation_id(v)


for key, value in data.items():
    if key == "definitions":
        for k, v in value.items():
            set_non_nullable(v)
    else:
        set_operation_id(value)

with open(path, "w") as f:
    json.dump(data, f, indent=2)
