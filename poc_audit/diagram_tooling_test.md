# Diagram tooling smoke test

Not a design artifact. The participants and messages below are illustrative
placeholders chosen to make the diagram wide. Nothing here is a design
position.

Purpose: let you check that your d2 previewer setup works, both for standalone
`.d2` files and for `d2` blocks embedded in markdown.

## What to install

- **d2, standalone files**: extension `terrastruct.d2`. Previews a `.d2` file
  on its own with `Ctrl+Shift+D`. Requires the `d2` binary on the machine where
  the extension runs, which under Remote-SSH is the workspace, not your laptop.
  Already installed here at `~/.local/share/mise/shims/d2`, version 0.7.1.
- **d2, inside markdown**: extension `kdheepak.d2-markdown-preview`. This is
  the one that renders fenced `d2` blocks in the markdown preview pane. The
  official `terrastruct.d2` extension advertises this but does not reliably
  deliver it: it works by reassigning `md.options.highlight`, which VS Code's
  own markdown renderer also sets, so whichever assigns last wins.

Two gotchas, both cost time to find:

1. `terrastruct.d2` probes for the `d2` binary at startup
   (`D2.checkForInstallAtStart` defaults to true). If you install `d2` after
   the editor is already running, the extension stays in a "not installed"
   state until you run **Developer: Reload Window**.
2. Setting `D2.execPath` to an absolute path is worth doing regardless, since
   the extension host's environment need not match your shell's.

## Test diagram

If this block renders below as a diagram rather than as source, your markdown
previewer is working. The sibling file `diagram_tooling_test.d2` holds the same
diagram for testing standalone preview.

The `auditable` class is applied to both messages and notes, so the render
also exercises d2 class support and lets the two markings be compared. Two
notes are left unmarked for contrast. Which items carry the class is
illustrative, not a design position.

```d2
classes: {
  auditable: {
    style: {
      stroke: "#B8860B"
      stroke-width: 3
      bold: true
      font-color: "#7A5C00"
      fill: "#FFF3C4"
    }
  }
}

seq: Sandbox creation (tooling smoke test) {
  shape: sequence_diagram

  user: User
  coderd: coderd
  prov: provisionerd
  wsagent: workspace_agent
  host: Sandbox host
  sandbox: Sandbox
  aiagent: AI agent
  journal: Audit journal

  user -> coderd: request sandbox

  # plain note, for contrast
  coderd.note: Authority of the principal is checked here

  # assignment of an identifier, occurs within the activation
  coderd.n2: sandbox identifier assigned { class: auditable }

  # database write
  coderd -> journal: entry, intent recorded { class: auditable }

  coderd -> prov: provision
  prov -> wsagent: create sandbox

  # creation of an entity
  wsagent -> host: start container { class: auditable }

  # creation of an entity, occurs within the activation
  host.n1: container entity created { class: auditable }

  host -> host: allocate resources

  outcome: {
    started: {
      host -> sandbox: boot { class: auditable }

      # assignment of an identifier, occurs within the activation
      sandbox.n1: AI agent identity issued { class: auditable }

      sandbox -> aiagent: launch { class: auditable }
      aiagent -> coderd: register, present identity { class: auditable }

      # database write
      coderd -> journal: entry, effect confirmed { class: auditable }

      # database write, occurs within the activation
      journal.n1: entry committed and ordered { class: auditable }
    }

    failed: {
      host -> wsagent: error
      wsagent -> coderd: report failure
      coderd -> journal: entry, intent abandoned { class: auditable }

      # plain note, for contrast
      journal.note: An orphan would be invisible here
    }
  }

  # sending a message outside the system
  coderd -> user: result { class: auditable }
}
```

## Measured

Eight participants, rendered by d2 at 1695 by 2620 pixels.

The dagre and elk layout engines produce byte-identical output here, because
d2 sequence diagrams use a dedicated sequence layout rather than a general
graph layout. Choosing between them therefore has no effect on this diagram
type.

At roughly 210 pixels per participant, twelve participants would be about
2500 pixels wide and sixteen about 3400.

Note that d2 has no content injection in its style vocabulary, so a class
cannot contribute a fixed prefix such as `audit:` to a label. Class marking is
purely visual and needs a legend or caption to be self-explanatory.
