/**
 * sideline status reporting for oh-my-pi.
 *
 * Reports this session as a task in the sideline sidebar: `working` while the
 * agent runs, `waiting` when it hands control back, `done` at the end of a
 * turn. The row is bound to this pane, so Enter in the sidebar jumps back here.
 *
 * The task id is $SIDELINE_TASK_ID when set, otherwise "<dir>-<pane>", which
 * keeps two sessions in the same directory from fighting over one row.
 *
 * This extension never touches the pane title: omp's own window status owns
 * it, and sideline only reads it. That is why there is no re-assert loop.
 *
 * Only an attended session reports. A headless `omp -p` started from this
 * pane (a smoke test run through a daemon) inherits TMUX_PANE, and an
 * in-process task subagent shares this module's env. Without the gate both
 * write rows bound to this pane: stray "<otherdir>-<pane>" rows that never
 * clear, and a subagent's `done` overwriting the parent's own row.
 */

import type { ExtensionAPI, ExtensionContext } from "@oh-my-pi/pi-coding-agent";
import { basename } from "node:path";

function attended(ctx: ExtensionContext): boolean {
  return ctx.hasUI === true && ctx.agent?.kind !== "sub";
}

export default function (pi: ExtensionAPI) {
  const pane = process.env.TMUX_PANE;
  // Outside tmux there is no pane to report against.
  if (!pane) {
    return;
  }

  let lastStatus: string | undefined;
  let queue = Promise.resolve();

  function taskID(): string {
    const explicit = process.env.SIDELINE_TASK_ID?.trim();
    if (explicit) {
      return explicit;
    }
    let dir = "session";
    try {
      dir = basename(process.cwd()) || "session";
    } catch {
      dir = "session";
    }
    return `${dir}-${pane.replace(/^%/, "")}`;
  }

  function sessionTitle(): string | undefined {
    try {
      const title = pi.getSessionName();
      return typeof title === "string" && title.trim() ? title.trim() : undefined;
    } catch {
      return undefined;
    }
  }

  function write(status: string): Promise<void> {
    const args = ["set", taskID(), "--status", status, "--bind-pane", pane];
    const title = sessionTitle();
    if (title) {
      args.push("--title", title);
    }
    // A missing or stopped sideline must never break the session.
    return pi.exec("sideline", args).then(
      () => {},
      () => {},
    );
  }

  function setStatus(status: string): Promise<void> {
    if (status === lastStatus) {
      return queue;
    }
    lastStatus = status;
    queue = queue.then(
      () => write(status),
      () => write(status),
    );
    return queue;
  }

  pi.on("session_start", async (_event, ctx) => {
    if (!attended(ctx)) return;
    await setStatus("waiting");
  });

  pi.on("agent_start", async (_event, ctx) => {
    if (!attended(ctx)) return;
    await setStatus("working");
  });

  pi.on("message_end", async (event, ctx) => {
    if (!attended(ctx)) return;
    if ("role" in event.message && event.message.role === "assistant") {
      await setStatus("waiting");
    }
  });

  pi.on("tool_call", async (event, ctx) => {
    if (!attended(ctx)) return;
    await setStatus(event.toolName === "ask" ? "waiting" : "working");
  });

  pi.on("tool_execution_start", async (_event, ctx) => {
    if (!attended(ctx)) return;
    await setStatus("working");
  });

  pi.on("agent_end", async (_event, ctx) => {
    if (!attended(ctx)) return;
    await setStatus("done");
  });
}
