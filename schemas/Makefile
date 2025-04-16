# To generate schema
# comment out this part from typescript-json-schema.js#L884
#
#	if (indexType.flags !== ts.TypeFlags.Number && !isIndexedObject) {
#			throw new Error("Not supported: IndexSignatureDeclaration with index symbol other than a number or a string");
#	}

gen-schema-single:
	bun -bun typescript-json-schema --noExtraProps --required --skipLibCheck --tsNodeRegister=true -o "${OUT}" "${IN}" ${CLASS}
	# minify
	python3 -c "import json; f=open('${OUT}', 'r'); j=json.load(f); f.close(); f=open('${OUT}', 'w'); json.dump(j, f, separators=(',', ':'));"

gen-schema:
	bun -bun tsc
	sed -i 's#"type": "module"#"type": "commonjs"#' package.json
	make IN=config/config.ts \
			CLASS=Config \
			OUT=config.schema.json \
			gen-schema-single
	make IN=providers/routes.ts \
			CLASS=Routes \
			OUT=routes.schema.json \
			gen-schema-single
	make IN=middlewares/middleware_compose.ts \
			CLASS=MiddlewareCompose \
			OUT=middleware_compose.schema.json \
			gen-schema-single
	make IN=docker.ts \
			CLASS=DockerRoutes \
			OUT=docker_routes.schema.json \
			gen-schema-single
	sed -i 's#"type": "commonjs"#"type": "module"#' package.json
	bun format:write