import { afterEach, describe, expect, test } from "bun:test";
import {
  mkdtemp,
  mkdir,
  readdir,
  readFile,
  rm,
  writeFile,
} from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import { rewriteImplMarkdown, syncImplDocs } from "./index";

describe("rewriteImplMarkdown", () => {
  test("uses a line anchor instead of appending the original fragment", () => {
    const rewritten = rewriteImplMarkdown({
      md: "# Feature\n\nFeature docs.\n\n## Usage\n\nSee [config](config.go:29#section).\n",
      pkgPath: "internal/feature",
      readmeRelToDocRoute: new Map(),
      dirPathToDocRoute: new Map(),
      repoUrl: "https://github.com/yusing/godoxy",
    });

    expect(rewritten).toContain(
      "https://github.com/yusing/godoxy/blob/main/internal/feature/config.go#L29",
    );
    expect(rewritten).not.toContain("#L29#section");
  });
});

describe("syncImplDocs", () => {
  const tempDirs: string[] = [];

  afterEach(async () => {
    while (tempDirs.length > 0) {
      const dir = tempDirs.pop();
      if (dir) {
        await rm(dir, { force: true, recursive: true });
      }
    }
  });

  test("ignores README files under scripts/", async () => {
    const repoRoot = await mkdtemp(path.join(os.tmpdir(), "update-wiki-repo-"));
    const wikiRoot = await mkdtemp(path.join(os.tmpdir(), "update-wiki-docs-"));
    tempDirs.push(repoRoot, wikiRoot);

    const scriptReadmeDir = path.join(repoRoot, "scripts", "minify");
    const includedReadmeDir = path.join(repoRoot, "internal", "feature");
    await mkdir(scriptReadmeDir, { recursive: true });
    await mkdir(includedReadmeDir, { recursive: true });
    await writeFile(
      path.join(scriptReadmeDir, "README.md"),
      ["# minify", "", "This README should be ignored."].join("\n"),
      "utf8",
    );
    await writeFile(
      path.join(includedReadmeDir, "README.md"),
      ["# Feature", "", "Feature docs.", "", "## Usage", "", "Hello.", ""].join(
        "\n",
      ),
      "utf8",
    );

    await syncImplDocs(repoRoot, wikiRoot);

    const implDir = path.join(wikiRoot, "content", "docs", "impl");
    const files = await readdir(implDir);
    expect(files).toContain("internal-feature.mdx");
    expect(files).not.toContain("scripts-minify.mdx");
  });

  test("writes missing mdx files and removes orphaned generated docs", async () => {
    const repoRoot = await mkdtemp(path.join(os.tmpdir(), "update-wiki-repo-"));
    const wikiRoot = await mkdtemp(path.join(os.tmpdir(), "update-wiki-docs-"));
    tempDirs.push(repoRoot, wikiRoot);

    const readmeDir = path.join(repoRoot, "internal", "feature");
    await mkdir(readmeDir, { recursive: true });
    await writeFile(
      path.join(readmeDir, "README.md"),
      [
        "# Feature",
        "",
        "Feature docs.",
        "",
        "## Usage",
        "",
        "See [config](config.go:29#section).",
        "",
      ].join("\n"),
      "utf8",
    );

    const implDir = path.join(wikiRoot, "content", "docs", "impl");
    await mkdir(implDir, { recursive: true });
    await writeFile(path.join(implDir, "orphan.mdx"), "stale", "utf8");

    await syncImplDocs(repoRoot, wikiRoot);

    const generated = path.join(implDir, "internal-feature.mdx");
    expect(await readFile(generated, "utf8")).toContain(
      "https://github.com/yusing/godoxy/blob/main/internal/feature/config.go#L29",
    );

    const files = await readdir(implDir);
    expect(files).toContain("internal-feature.mdx");
    expect(files).not.toContain("orphan.mdx");
  });
});
