import type {
	ExtensionAPI,
	ExtensionContext,
} from "@earendil-works/pi-coding-agent";
import { Type } from "typebox";
import { spawn } from "node:child_process";

const HLEDIT_BIN =
	process.env.HLEDIT_BIN || `${process.env.HOME}/.local/bin/hledit`;

const ReadParams = Type.Object({
	path: Type.String({ description: "File path to read" }),
	offset: Type.Optional(
		Type.Number({ description: "1-indexed starting line; for read-range" }),
	),
	limit: Type.Optional(
		Type.Number({ description: "Maximum lines to return; for read-range" }),
	),
	grep: Type.Optional(
		Type.String({
			description: "Filter lines by substring match (reduces token usage)",
		}),
	),
});

const ReplaceParams = Type.Object({
	path: Type.String({ description: "File path to edit" }),
	anchor: Type.String({ description: "Hashline anchor, e.g. 12#NK" }),
	content: Type.String({
		description: "Replacement content. Empty string deletes the line.",
	}),
});

const ReplaceRangeParams = Type.Object({
	path: Type.String({ description: "File path to edit" }),
	anchor: Type.String({ description: "Start anchor, inclusive, e.g. 12#NK" }),
	endAnchor: Type.String({ description: "End anchor, inclusive, e.g. 18#VR" }),
	content: Type.String({
		description: "Replacement content. Empty string deletes the range.",
	}),
});

const InsertParams = Type.Object({
	path: Type.String({ description: "File path to edit" }),
	anchor: Type.String({
		description: "Anchor to insert before/after, e.g. 12#NK",
	}),
	content: Type.String({
		description: "Content to insert. Must be non-empty.",
	}),
	after: Type.Optional(
		Type.Boolean({
			description: "If true, insert after the anchor. Default inserts before.",
		}),
	),
});

type HleditRun = {
	stdout: string;
	stderr: string;
	exitCode: number | null;
};

async function runHledit(
	args: string[],
	stdin: string | undefined,
	ctx: ExtensionContext,
): Promise<HleditRun> {
	return new Promise((resolve, reject) => {
		const child = spawn(HLEDIT_BIN, args, {
			cwd: ctx.cwd,
			signal: ctx.signal,
			stdio: ["pipe", "pipe", "pipe"],
		});

		let stdout = "";
		let stderr = "";

		child.stdout.setEncoding("utf8");
		child.stderr.setEncoding("utf8");
		child.stdout.on("data", (chunk) => {
			stdout += chunk;
		});
		child.stderr.on("data", (chunk) => {
			stderr += chunk;
		});
		child.on("error", resolve);
		child.on("close", (exitCode) => {
			resolve({ stdout, stderr, exitCode });
		});

		child.stdin.end(stdin ?? "");
	});
}

function textResult(label: string, run: HleditRun, args: string[]) {
	const text = run.stdout.trimEnd();
	return {
		content: [{ type: "text" as const, text }],
		details: { ok: run.exitCode === 0, label },
		isError: run.exitCode !== 0,
	};
}

function toNum(v: number | undefined): number | undefined {
	return v !== undefined && v >= 0 ? v : undefined;
}

export default function piHleditExtension(pi: ExtensionAPI) {
	pi.registerTool({
		name: "hledit_read",
		label: "Hashline Read",
		description:
			"Read a text file with LN#HASH anchors for stale-safe editing. Use this before hledit_replace, hledit_replace_range, or hledit_insert.",
		promptSnippet: "Read files with LN#HASH anchors for stale-safe editing",
		promptGuidelines: [
			"Use hledit_read before using hledit_replace, hledit_replace_range, or hledit_insert.",
			"Prefer hledit_* tools over text replacement when editing files because hash anchors detect stale context.",
		],
		parameters: ReadParams,
		async execute(_toolCallId, params, _signal, _onUpdate, ctx) {
			const offset = toNum(params.offset);
			const limit = toNum(params.limit);
			const grep = (params.grep as string) || undefined;
			const hasRange =
				offset !== undefined || limit !== undefined || grep !== undefined;
			const args = hasRange
				? [
						"read-range",
						params.path,
						"--offset",
						String(offset ?? 1),
						"--limit",
						String(limit ?? 2000),
						...(grep ? ["--grep", grep] : []),
					]
				: ["read", params.path];
			const run = await runHledit(args, undefined, ctx);
			return textResult("hledit_read", run, args);
		},
	});

	pi.registerTool({
		name: "hledit_replace",
		label: "Hashline Replace",
		description:
			"Replace or delete one line using an LN#HASH anchor. Content is passed via stdin to hledit.",
		promptSnippet: "Replace/delete one anchored line after hledit_read",
		promptGuidelines: [
			"Use hledit_replace only with an anchor returned by a recent hledit_read call.",
			"If hledit_replace returns stale, retry with the returned current anchor or re-read the file.",
		],
		parameters: ReplaceParams,
		async execute(_toolCallId, params, _signal, _onUpdate, ctx) {
			const args = ["replace", params.path, params.anchor, "-"];
			const run = await runHledit(args, params.content, ctx);
			return textResult("hledit_replace", run, args);
		},
	});

	pi.registerTool({
		name: "hledit_replace_range",
		label: "Hashline Replace Range",
		description:
			"Replace or delete a line range using start/end LN#HASH anchors. Content is passed via stdin to hledit.",
		promptSnippet: "Replace/delete an anchored range after hledit_read",
		promptGuidelines: [
			"Use hledit_replace_range only with anchors returned by a recent hledit_read call.",
			"Use empty content with hledit_replace_range to delete a range.",
		],
		parameters: ReplaceRangeParams,
		async execute(_toolCallId, params, _signal, _onUpdate, ctx) {
			const args = [
				"replace-range",
				params.path,
				params.anchor,
				params.endAnchor,
				"-",
			];
			const run = await runHledit(args, params.content, ctx);
			return textResult("hledit_replace_range", run, args);
		},
	});

	pi.registerTool({
		name: "hledit_insert",
		label: "Hashline Insert",
		description:
			"Insert content before or after a line using an LN#HASH anchor. Content is passed via stdin to hledit.",
		promptSnippet:
			"Insert content before/after an anchored line after hledit_read",
		promptGuidelines: [
			"Use hledit_insert only with an anchor returned by a recent hledit_read call.",
			"Set after=true to insert after the anchor; omit after to insert before.",
		],
		parameters: InsertParams,
		async execute(_toolCallId, params, _signal, _onUpdate, ctx) {
			const args = params.after
				? ["insert", "--after", params.path, params.anchor, "-"]
				: ["insert", "--before", params.path, params.anchor, "-"];
			const run = await runHledit(args, params.content, ctx);
			return textResult("hledit_insert", run, args);
		},
	});

	pi.registerCommand("hledit-status", {
		description:
			"Check the configured hledit binary used by the pi-hledit extension",
		handler: async (_args, ctx) => {
			const run = await runHledit(["help"], undefined, ctx);
			if (run.exitCode === 0) {
				ctx.ui.notify(`hledit ready: ${HLEDIT_BIN}`, "info");
			} else {
				ctx.ui.notify(
					`hledit failed: ${HLEDIT_BIN}\n${run.stderr || run.stdout}`,
					"error",
				);
			}
		},
	});

	pi.registerTool({
		name: "hledit_batch",
		label: "Hashline Batch Edit",
		description:
			"Apply multiple edit operations atomically. All anchors are validated against the same file state, then edits are applied bottom-up in a single write. Use this when making multiple edits to avoid stale anchors between operations.",
		promptSnippet:
			"Apply multiple hashline edits atomically in a single operation",
		promptGuidelines: [
			"Use hledit_batch when making multiple edits to the same file in one turn.",
			"All anchors must come from the same hledit_read call.",
			"If hledit_batch returns stale, re-read the file and retry with fresh anchors.",
			"Operations: replace (swap line), delete (remove line), insert (add before line).",
		],
		parameters: Type.Object({
			path: Type.String({ description: "File path to edit" }),
			edits: Type.Array(
				Type.Object({
					op: Type.String({
						description: "Operation: replace, delete, or insert",
					}),
					pos: Type.String({ description: "Anchor, e.g. 12#NK" }),
					end_pos: Type.Optional(
						Type.String({ description: "End anchor for replace_range" }),
					),
					lines: Type.Optional(
						Type.Array(Type.String(), {
							description: "Replacement lines; empty = delete",
						}),
					),
				}),
				{ description: "Array of edit operations" },
			),
		}),
		async execute(_toolCallId, params, _signal, _onUpdate, ctx) {
			const batchReq = {
				edits: (
					params.edits as Array<{
						op: string;
						pos: string;
						end_pos?: string;
						lines?: Array<string>;
					}>
				).map((e) => ({
					op: e.op,
					pos: e.pos,
					...(e.end_pos ? { end_pos: e.end_pos } : {}),
					lines: e.lines ?? [],
				})),
			};
			const stdin = JSON.stringify(batchReq);
			const args = ["batch", params.path];
			const run = await runHledit(args, stdin, ctx);
			return textResult("hledit_batch", run, args);
		},
	});
}
