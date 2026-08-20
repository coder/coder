ALTER TABLE mcp_server_configs
	ADD COLUMN oauth2_issuer text NOT NULL DEFAULT '';

COMMENT ON COLUMN mcp_server_configs.oauth2_issuer IS 'Authorization server issuer identifier (RFC 8414) recorded during OAuth2 discovery. Client credentials are bound to this issuer (MCP 2026-07-28, SEP-2352) and it is compared against the iss authorization response parameter (RFC 9207, SEP-2468). Empty when credentials were configured manually and discovery has not succeeded; rows created before this column existed are backfilled lazily at OAuth2 connect time.';

ALTER TABLE mcp_server_configs
	ADD COLUMN oauth2_iss_required boolean NOT NULL DEFAULT false;

COMMENT ON COLUMN mcp_server_configs.oauth2_iss_required IS 'True when the authorization server metadata advertised authorization_response_iss_parameter_supported. RFC 9207 then requires authorization responses to carry a matching iss parameter; responses without one are rejected.';
