-- Script order rules can skip scripts whose dependencies did not succeed.
ALTER TYPE workspace_agent_script_timing_status ADD VALUE IF NOT EXISTS 'skipped';
