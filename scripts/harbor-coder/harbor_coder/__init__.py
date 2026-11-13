"""Harbor integration for Coder.

Two pieces, both loadable by import path with no Harbor changes:

- ``harbor_coder.environment:CoderEnvironment`` runs each Harbor trial inside
  a Coder workspace (``--env harbor_coder.environment:CoderEnvironment``).
- ``harbor_coder.agent:CoderAgentsAgent`` drives Coder Agents (the coderd chat
  agent) against that workspace (``--agent harbor_coder.agent:CoderAgentsAgent``).
"""
