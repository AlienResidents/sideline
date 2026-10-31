/**
 * sideline status reporting for oh-my-pi.
 *
 * Reports the session as a task row: `working` while the agent runs, `waiting`
 * when it hands control back, `done` when the turn ends. The row is bound to
 * the pane the session runs in, so `enter` in the sidebar jumps back here.
 *
 * The task id is $SIDELINE_TASK_ID when set, otherwise the working directory
 * basename. Everything else about the row lives in the sideline state file.
 *
 * Unlike a sidebar that owns pane titles, sideline never rewrites them, so
 * there is no title re-assert loop here: one write per status change is
 * enough.
 *
 * Copy to ~/.omp/agent/extensions/ and reload the session.
 */

import type { ExtensionAPI } from "@oh-my-pi/pi-coding-agent";
import { basename } from "node:path";

export default function (pi: ExtensionAPI) {
  let lastStatus: string | undefined;
  let queue = Promise.resolve();

  function taskID(): string {
    const fromEnv = process.env.SIDELINE_TASK_ID?.trim();
    if (fromEnv) return fromEnv;
    try {
      return basename(process.cwd()) || "session";
    } catch {
      return "session";
    }
  }

  function sessionTitle(): string | undefined {
    try {
      const title = pi.getSessionName();
      return typeof title === "string" && title.trim() ? title.trim() : undefined;
    } catch {
      return undefined;
    }
  }

  // The pane title is what the sidebar shows in its pane column; the task
  // title is what it shows in the roster. Keep both truthful.
  function write(status: string): Promise<void> {
    const id = taskID();
    const args = ["set", id, "--status", status];

    const title = sessionTitle();
    if (title) {
      args.push("--title", title);
    }
    const pane = process.env.TMUX_PANE;
    if (pane) {
      args.push("--bind-pane", pane);
    }

    return pi
      .exec("sideline", args)
      .then(
        () => setPaneTitle(status),
        () => setPaneTitle(status),
      )
      .then(
        () => {},
        () => {},
      );
  }

  function setPaneTitle(status: string): Promise<void> {
    const pane = process.env.TMUX_PANE;
    const title = sessionTitle();
    if (!pane || !title) {
      return Promise.resolve();
    }
    return pi
      .exec("tmux", ["select-pane", "-t", pane, "-T", `${title} - ${status}`])
      .then(
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

  pi.on("session_start", async () => {
    await setStatus("waiting");
  });

  pi.on("agent_start", async () => {
    await setStatus("working");
  });

  pi.on("message_end", async (event) => {
    if ("role" in event.message && event.message.role === "assistant") {
      await setStatus("waiting");
    }
  });

  pi.on("tool_call", async (event) => {
    await setStatus(event.toolName === "ask" ? "waiting" : "working");
  });

  pi.on("tool_execution_start", async () => {
    await setStatus("working");
  });

  pi.on("agent_end", async () => {
    if (lastStatus === "done") {
      return;
    }
    lastStatus = "done";
    await write("done");
  });
}
