import { Glob } from "bun";
import { linkSync } from "fs";
import { mkdir, readdir, readFile, rm, writeFile } from "fs/promises";
import path from "path";

type ImplDoc = {
  /** Directory path relative to this repo, e.g. "internal/health/check" */
  pkgPath: string;
  /** File name in wiki `src/impl/`, e.g. "internal-health-check.md" */
  docFileName: string;
  /** Absolute source README path */
  srcPathAbs: string;
  /** Absolute destination doc path */
  dstPathAbs: string;
};

const START_MARKER = "// GENERATED-IMPL-SIDEBAR-START";
const END_MARKER = "// GENERATED-IMPL-SIDEBAR-END";

function escapeRegex(s: string) {
  return s.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

function escapeSingleQuotedTs(s: string) {
  return s.replace(/\\/g, "\\\\").replace(/'/g, "\\'");
}

function sanitizeFileStemFromPkgPath(pkgPath: string) {
  // Convert a package path into a stable filename.
  // Example: "internal/go-oidc/example" -> "internal-go-oidc-example"
  // Keep it readable and unique (uses full path).
  const parts = pkgPath
    .split("/")
    .filter(Boolean)
    .map((p) => p.replace(/[^A-Za-z0-9._-]+/g, "-"));
  const joined = parts.join("-");
  return joined.replace(/-+/g, "-").replace(/^-|-$/g, "");
}

async function listRepoReadmes(repoRootAbs: string): Promise<string[]> {
  const glob = new Glob("**/README.md");
  const readmes: string[] = [];

  for await (const rel of glob.scan({
    cwd: repoRootAbs,
    onlyFiles: true,
    dot: false,
  })) {
    // Bun returns POSIX-style rel paths.
    if (rel === "README.md") continue; // exclude root README
    if (rel.startsWith(".git/") || rel.includes("/.git/")) continue;
    if (rel.startsWith("node_modules/") || rel.includes("/node_modules/"))
      continue;
    if (rel.startsWith("internal/go-oidc/")) continue;
    if (rel.startsWith("internal/gopsutil/")) continue;
    readmes.push(rel);
  }

  // Deterministic order.
  readmes.sort((a, b) => a.localeCompare(b));
  return readmes;
}

async function ensureHardLink(srcAbs: string, dstAbs: string) {
  await mkdir(path.dirname(dstAbs), { recursive: true });
  await rm(dstAbs, { force: true });
  // Prefer sync for better error surfaces in Bun on some platforms.
  linkSync(srcAbs, dstAbs);
}

async function syncImplDocs(
  repoRootAbs: string,
  wikiRootAbs: string
): Promise<ImplDoc[]> {
  const implDirAbs = path.join(wikiRootAbs, "src", "impl");
  await mkdir(implDirAbs, { recursive: true });

  const readmes = await listRepoReadmes(repoRootAbs);
  const docs: ImplDoc[] = [];
  const expectedFileNames = new Set<string>();

  for (const readmeRel of readmes) {
    const pkgPath = path.posix.dirname(readmeRel);
    if (!pkgPath || pkgPath === ".") continue;

    const docStem = sanitizeFileStemFromPkgPath(pkgPath);
    if (!docStem) continue;
    const docFileName = `${docStem}.md`;

    const srcPathAbs = path.join(repoRootAbs, readmeRel);
    const dstPathAbs = path.join(implDirAbs, docFileName);

    await ensureHardLink(srcPathAbs, dstPathAbs);

    docs.push({ pkgPath, docFileName, srcPathAbs, dstPathAbs });
    expectedFileNames.add(docFileName);
  }

  // Clean orphaned impl docs.
  const existing = await readdir(implDirAbs, { withFileTypes: true });
  for (const ent of existing) {
    if (!ent.isFile()) continue;
    if (!ent.name.endsWith(".md")) continue;
    if (expectedFileNames.has(ent.name)) continue;
    await rm(path.join(implDirAbs, ent.name), { force: true });
  }

  // Deterministic for sidebar.
  docs.sort((a, b) => a.pkgPath.localeCompare(b.pkgPath));
  return docs;
}

function renderSidebarItems(docs: ImplDoc[], indent: string) {
  // link: '/impl/<file>.md' because VitePress `srcDir = "src"`.
  if (docs.length === 0) return "";
  return (
    docs
      .map((d) => {
        const text = escapeSingleQuotedTs(d.pkgPath);
        const link = escapeSingleQuotedTs(`/impl/${d.docFileName}`);
        return `${indent}{ text: '${text}', link: '${link}' },`;
      })
      .join("\n") + "\n"
  );
}

async function updateVitepressSidebar(wikiRootAbs: string, docs: ImplDoc[]) {
  const configPathAbs = path.join(wikiRootAbs, ".vitepress", "config.mts");
  if (!(await Bun.file(configPathAbs).exists())) {
    throw new Error(`vitepress config not found: ${configPathAbs}`);
  }

  const original = await readFile(configPathAbs, "utf8");

  // Replace between markers with generated items.
  // We keep indentation based on the marker line.
  const markerRe = new RegExp(
    `(^[\\t ]*)${escapeRegex(START_MARKER)}[\\s\\S]*?\\n\\1${escapeRegex(
      END_MARKER
    )}`,
    "m"
  );

  const m = original.match(markerRe);
  if (!m) {
    throw new Error(
      `sidebar markers not found in ${configPathAbs}. Expected lines: ${START_MARKER} ... ${END_MARKER}`
    );
  }
  const indent = m[1] ?? "";
  const generated = `${indent}${START_MARKER}\n${renderSidebarItems(
    docs,
    indent
  )}${indent}${END_MARKER}`;

  const updated = original.replace(markerRe, generated);
  if (updated !== original) {
    await writeFile(configPathAbs, updated);
  }
}

async function main() {
  // This script lives in `scripts/update-wiki/`, so repo root is two levels up.
  const repoRootAbs = path.resolve(import.meta.dir, "../..");

  // Required by task, but allow overriding via env for convenience.
  const wikiRootAbs = Bun.env.DOCS_DIR
    ? path.resolve(repoRootAbs, Bun.env.DOCS_DIR)
    : path.resolve(repoRootAbs, "..", "godoxy-webui", "wiki");

  const docs = await syncImplDocs(repoRootAbs, wikiRootAbs);
  await updateVitepressSidebar(wikiRootAbs, docs);
}

await main();
