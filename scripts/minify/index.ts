import { glob } from "node:fs";
import { transform as transformJS } from "@swc/core";
import { minify as minifyHTML } from "@swc/html";
import { basename, dirname, join } from "node:path";

type Kind = "html" | "js";

function isIgnored(path: string): boolean {
  return (
    path.startsWith("internal/go-proxmox") || basename(path).includes(".min.")
  );
}

function globAssets(extension: Kind): Promise<string[]> {
  return new Promise((resolve, reject) => {
    glob(
      [`internal/**/*.${extension}`, `goutils/**/*.${extension}`],
      (err, matches) => {
        if (err) {
          reject(err);
          return;
        }

        resolve(matches.filter((match) => !isIgnored(match)));
      },
    );
  });
}

async function minify(filePath: string, kind: Kind): Promise<string> {
  const content = await Bun.file(filePath).text();
  if (kind === "js") {
    const out = await transformJS(content, {
      sourceMaps: false,
      isModule: false,
      minify: true,
    });
    if (!out.code) {
      throw new Error(`js minify output is empty for "${filePath}"`);
    }
    return out.code;
  }
  const out = await minifyHTML(content, {
    forceSetHtml5Doctype: true,
    collapseBooleanAttributes: true,
    collapseWhitespaces: "all",
    minifyCss: { lib: "lightningcss" },
    minifyJs: true,
    removeComments: true,
    removeEmptyMetadataElements: true,
  });
  if (out.errors && out.errors.length > 0) {
    const err = `html minify error for "${filePath}": ${out.errors.map((e) => e.message)}`;
    if (!out.code) {
      throw new Error(err);
    }
    console.error(err);
  }
  return out.code;
}

async function minifyOut(filePath: string, kind: Kind): Promise<void> {
  const minified = await minify(filePath, kind);

  const fnameNoExt = basename(filePath).split(".")[0]!;
  const outPath = join(dirname(filePath), `${fnameNoExt}.min.${kind}`);

  console.log(`minify ${filePath} -> ${outPath}`);
  await Bun.file(outPath).write(minified);
}

async function main() {
  const [htmlFiles, jsFiles] = await Promise.all([
    globAssets("html"),
    globAssets("js"),
  ]);

  await Promise.all([
    ...htmlFiles.map((filePath) => minifyOut(filePath, "html")),
    ...jsFiles.map((filePath) => minifyOut(filePath, "js")),
  ]);
}

await main();
