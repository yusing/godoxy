# This script aims to fix the swagger.json file by setting the x-nullable flag to False if not present for all objects and arrays.
# This prevents from generating optional (undefined) fields in the generated API client.
import json

path = "internal/api/v1/docs/swagger.json"

with open(path, "r") as f:
    data = json.load(f)

def set_non_nullable(data):
    if not isinstance(data, dict):
        return
    if "x-nullable" not in data:
        data["x-nullable"] = False
    if "x-omitempty" not in data and data["x-nullable"] == False:
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