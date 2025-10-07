# AI Bridge Grafana Dashboard

A sample Grafana dashboard for monitoring AI Bridge token usage, costs, and cache hit rates in Coder.

![AI Bridge example Grafana Dashboard](https://github.com/user-attachments/assets/c33d5028-265f-4add-9def-fe3a56a785f1)


## Setup

1. **Install the Infinity plugin**: `grafana-cli plugins install yesoreyeram-infinity-datasource`

2. **Configure data sources**:
   - **PostgreSQL datasource** (`coder-observability-ro`): Connect to your Coder database with read access to `aibridge_token_usages`, `aibridge_interceptions`, and `users` tables
   - **Infinity datasource** (`litellm-pricing-data`): Point to `https://raw.githubusercontent.com/BerriAI/litellm/refs/heads/main/model_prices_and_context_window.json` for model pricing data

3. **Import**: Download `dashboard.json` from this directory, then in Grafana navigate to **Dashboards** → **Import** → **Upload JSON file**. Map the data sources when prompted.

## Features

- Token usage leaderboards by user, provider, and model
- Filterable by time range, username, provider, and model (regex supported)
