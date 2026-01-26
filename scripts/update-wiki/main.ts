import { Glob } from "bun";
import { mkdir, readdir, readFile, rm, writeFile } from "fs/promises";
import path from "path";

type ImplDoc = {
  /** Directory path relative to this repo, e.g. "internal/health/check" */
  pkgPath: string;
  /** File name in wiki `src/impl/`, e.g. "internal-health-check.md" */
  docFileName: string;
  /** VitePress route path (extensionless), e.g. "/impl/internal-health-check" */
  docRoute: string;
  /** Absolute source README path */
  srcPathAbs: string;
  /** Absolute destination doc path */
  dstPathAbs: string;
};

const START_MARKER = "// GENERATED-IMPL-SIDEBAR-START";
const END_MARKER = "// GENERATED-IMPL-SIDEBAR-END";

const skipSubmodules = ["internal/go-oidc/", "internal/gopsutil/", "internal/go-proxmox/"];

function escapeRegex(s: string) {
  return s.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

function escapeSingleQuotedTs(s: string) {
  return s.replace(/\\/g, "\\\\").replace(/'/g, "\\'");
}

function normalizeRepoUrl(raw: string) {
  let url = (raw ?? "").trim();
  if (!url) return "";
  // Common typo: "https://https://github.com/..."
  url = url.replace(/^https?:\/\/https?:\/\//i, "https://");
  if (!/^https?:\/\//i.test(url)) url = `https://${url}`;
  url = url.replace(/\/+$/, "");
  return url;
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

function splitUrlAndFragment(url: string): {
  urlNoFragment: string;
  fragment: string;
} {
  const i = url.indexOf("#");
  if (i === -1) return { urlNoFragment: url, fragment: "" };
  return { urlNoFragment: url.slice(0, i), fragment: url.slice(i) };
}

function isExternalOrAbsoluteUrl(url: string) {
  // - absolute site links: "/foo"
  // - pure fragments: "#bar"
  // - external schemes: "https:", "mailto:", "vscode:", etc.
  //   IMPORTANT: don't treat "config.go:29" as a scheme.
  if (url.startsWith("/") || url.startsWith("#")) return true;
  if (url.includes("://")) return true;
  return /^(https?|mailto|tel|vscode|file|data|ssh|git):/i.test(url);
}

function isRepoSourceFilePath(filePath: string) {
  // Conservative allow-list: avoid rewriting .md (non-README) which may be VitePress docs.
  return /\.(go|ts|tsx|js|jsx|py|sh|yml|yaml|json|toml|env|css|html|txt)$/i.test(
    filePath
  );
}

function parseFileLineSuffix(urlNoFragment: string): {
  filePath: string;
  line?: string;
} {
  // Match "file.ext:123" (line suffix), while leaving "file.ext" untouched.
  const m = urlNoFragment.match(/^(.*?):(\d+)$/);
  if (!m) return { filePath: urlNoFragment };
  return { filePath: m[1] ?? urlNoFragment, line: m[2] };
}

function rewriteMarkdownLinksOutsideFences(
  md: string,
  rewriteInline: (url: string) => string
) {
  const lines = md.split("\n");
  let inFence = false;

  for (let i = 0; i < lines.length; i++) {
    const line = lines[i] ?? "";
    const trimmed = line.trimStart();
    if (trimmed.startsWith("```")) {
      inFence = !inFence;
      continue;
    }
    if (inFence) continue;

    // Inline markdown links/images: [text](url "title") / ![alt](url)
    lines[i] = line.replace(
      /\]\(([^)\s]+)(\s+"[^"]*")?\)/g,
      (_full, urlRaw: string, maybeTitle: string | undefined) => {
        const rewritten = rewriteInline(urlRaw);
        return `](${rewritten}${maybeTitle ?? ""})`;
      }
    );
  }

  return lines.join("\n");
}

function rewriteImplMarkdown(params: {
  md: string;
  pkgPath: string;
  readmeRelToDocRoute: Map<string, string>;
  dirPathToDocRoute: Map<string, string>;
  repoUrl: string;
}) {
  const { md, pkgPath, readmeRelToDocRoute, dirPathToDocRoute, repoUrl } =
    params;

  return rewriteMarkdownLinksOutsideFences(md, (urlRaw) => {
    // Handle angle-bracketed destinations: (<./foo/README.md>)
    const angleWrapped =
      urlRaw.startsWith("<") && urlRaw.endsWith(">")
        ? urlRaw.slice(1, -1)
        : urlRaw;

    const { urlNoFragment, fragment } = splitUrlAndFragment(angleWrapped);
    if (!urlNoFragment) return urlRaw;
    if (isExternalOrAbsoluteUrl(urlNoFragment)) return urlRaw;

    // 1) Directory links like "common" or "common/" that have a README
    const dirPathNormalized = urlNoFragment.replace(/\/+$/, "");
    if (dirPathToDocRoute.has(dirPathNormalized)) {
      const rewritten = `${dirPathToDocRoute.get(
        dirPathNormalized
      )!}${fragment}`;
      return angleWrapped === urlRaw ? rewritten : `<${rewritten}>`;
    }

    // 2) Intra-repo README links -> VitePress impl routes
    if (/(^|\/)README\.md$/.test(urlNoFragment)) {
      const targetReadmeRel = path.posix.normalize(
        path.posix.join(pkgPath, urlNoFragment)
      );
      const route = readmeRelToDocRoute.get(targetReadmeRel);
      if (route) {
        const rewritten = `${route}${fragment}`;
        return angleWrapped === urlRaw ? rewritten : `<${rewritten}>`;
      }
      return urlRaw;
    }

    // 3) Local source-file references like "config.go:29" -> GitHub blob link
    if (repoUrl) {
      const { filePath, line } = parseFileLineSuffix(urlNoFragment);
      if (isRepoSourceFilePath(filePath)) {
        const repoRel = path.posix.normalize(
          path.posix.join(pkgPath, filePath)
        );
        const githubUrl = `${repoUrl}/blob/main/${repoRel}${line ? `#L${line}` : ""
          }`;
        const rewritten = `${githubUrl}${fragment}`;
        return angleWrapped === urlRaw ? rewritten : `<${rewritten}>`;
      }
    }

    return urlRaw;
  });
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
    let skip = false;
    for (const submodule of skipSubmodules) {
      if (rel.startsWith(submodule)) {
        skip = true;
        break;
      }
    }
    if (skip) continue;
    readmes.push(rel);
  }

  // Deterministic order.
  readmes.sort((a, b) => a.localeCompare(b));
  return readmes;
}

async function writeImplDocCopy(params: {
  srcAbs: string;
  dstAbs: string;
  pkgPath: string;
  readmeRelToDocRoute: Map<string, string>;
  dirPathToDocRoute: Map<string, string>;
  repoUrl: string;
}) {
  const {
    srcAbs,
    dstAbs,
    pkgPath,
    readmeRelToDocRoute,
    dirPathToDocRoute,
    repoUrl,
  } = params;
  await mkdir(path.dirname(dstAbs), { recursive: true });
  await rm(dstAbs, { force: true });

  const original = await readFile(srcAbs, "utf8");
  const rewritten = rewriteImplMarkdown({
    md: original,
    pkgPath,
    readmeRelToDocRoute,
    dirPathToDocRoute,
    repoUrl,
  });
  await writeFile(dstAbs, rewritten);
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
  expectedFileNames.add("introduction.md");

  const repoUrl = normalizeRepoUrl(
    Bun.env.REPO_URL ?? "https://github.com/yusing/godoxy"
  );

  // Precompute mapping from repo-relative README path -> VitePress route.
  // This lets us rewrite intra-repo README links when copying content.
  const readmeRelToDocRoute = new Map<string, string>();

  // Also precompute mapping from directory path -> VitePress route.
  // This handles links like "[`common/`](common)" that point to directories with READMEs.
  const dirPathToDocRoute = new Map<string, string>();

  for (const readmeRel of readmes) {
    const pkgPath = path.posix.dirname(readmeRel);
    if (!pkgPath || pkgPath === ".") continue;

    const docStem = sanitizeFileStemFromPkgPath(pkgPath);
    if (!docStem) continue;
    const route = `/impl/${docStem}`;
    readmeRelToDocRoute.set(readmeRel, route);
    dirPathToDocRoute.set(pkgPath, route);
  }

  for (const readmeRel of readmes) {
    const pkgPath = path.posix.dirname(readmeRel);
    if (!pkgPath || pkgPath === ".") continue;

    const docStem = sanitizeFileStemFromPkgPath(pkgPath);
    if (!docStem) continue;
    const docFileName = `${docStem}.md`;
    const docRoute = `/impl/${docStem}`;

    const srcPathAbs = path.join(repoRootAbs, readmeRel);
    const dstPathAbs = path.join(implDirAbs, docFileName);

    await writeImplDocCopy({
      srcAbs: srcPathAbs,
      dstAbs: dstPathAbs,
      pkgPath,
      readmeRelToDocRoute,
      dirPathToDocRoute,
      repoUrl,
    });

    docs.push({ pkgPath, docFileName, docRoute, srcPathAbs, dstPathAbs });
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
  // link: '/impl/<stem>' (extensionless) because VitePress `srcDir = "src"`.
  if (docs.length === 0) return "";
  return (
    docs
      .map((d) => {
        const text = escapeSingleQuotedTs(d.pkgPath);
        const link = escapeSingleQuotedTs(d.docRoute);
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
