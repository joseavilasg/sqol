import { execFile } from "node:child_process";
import { chmodSync, copyFileSync, existsSync, mkdirSync } from "node:fs";
import { join } from "node:path";
import * as vscode from "vscode";

/** The Go target this editor runs on, spelled the way the binaries are named. */
function target(): string {
  const os = process.platform === "win32" ? "windows" : process.platform;
  const arch = process.arch === "x64" ? "amd64" : process.arch;
  return `${os}-${arch}`;
}

/** The file name of the binary for this platform. */
function binaryName(): string {
  const suffix = process.platform === "win32" ? ".exe" : "";
  return `sqol-${target()}${suffix}`;
}

/**
 * Copies the bundled binary somewhere the system does not lock, because Windows
 * refuses to update or remove an executable while a process runs from it, and
 * the extension's own folder lives under the application's install path.
 */
function unlock(context: vscode.ExtensionContext, bundled: string): string {
  if (process.platform !== "win32") {
    chmodSync(bundled, 0o755);
    return bundled;
  }

  const version = context.extension.packageJSON.version as string;
  const folder = join(context.globalStorageUri.fsPath, version);
  mkdirSync(folder, { recursive: true });

  const copy = join(folder, binaryName());
  if (!existsSync(copy)) {
    copyFileSync(bundled, copy);
  }
  return copy;
}

/** Resolves the binary to run: the setting, the bundled one, or the PATH. */
function resolve(context: vscode.ExtensionContext): string {
  const configured = vscode.workspace
    .getConfiguration("sqol")
    .get<string>("path", "")
    .trim();
  if (configured) {
    return configured;
  }

  const bundled = join(context.extensionUri.fsPath, "bin", binaryName());
  if (existsSync(bundled)) {
    return unlock(context, bundled);
  }

  return "sqol";
}

/** Runs the formatter over one document and returns its output. */
function format(binary: string, source: string): Promise<string> {
  return new Promise((resolve, reject) => {
    const child = execFile(
      binary,
      ["-stdin"],
      { timeout: 10_000, maxBuffer: 64 * 1024 * 1024, encoding: "utf8" },
      (error, stdout, stderr) => {
        if (error) {
          reject(new Error(stderr.trim() || error.message));
          return;
        }
        resolve(stdout);
      },
    );
    child.stdin?.end(source);
  });
}

/** The provider that answers Format Document for a SQL file. */
function provider(
  context: vscode.ExtensionContext,
): vscode.DocumentFormattingEditProvider {
  return {
    async provideDocumentFormattingEdits(document) {
      const source = document.getText();
      let formatted: string;

      try {
        formatted = await format(resolve(context), source);
      } catch (error) {
        const detail = error instanceof Error ? error.message : String(error);
        void vscode.window.showErrorMessage(`SQoL: ${detail}`);
        return [];
      }

      if (formatted === source) {
        return [];
      }

      const whole = new vscode.Range(
        document.positionAt(0),
        document.positionAt(source.length),
      );
      return [vscode.TextEdit.replace(whole, formatted)];
    },
  };
}

export function activate(context: vscode.ExtensionContext): void {
  context.subscriptions.push(
    vscode.languages.registerDocumentFormattingEditProvider(
      { language: "sql" },
      provider(context),
    ),
  );
}

export function deactivate(): void {
  // Nothing to release: every run is one process that ends with its answer.
}
