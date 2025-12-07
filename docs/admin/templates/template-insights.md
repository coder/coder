# Template Insights

Template Insights provides detailed analytics and usage metrics for your Coder templates. This feature helps administrators and template owners understand how templates are being used, identify popular applications, and monitor user engagement.

## Overview

Template Insights offers visibility into:

- **Active Users**: Track the number of users actively using workspaces from a template
- **Application Usage**: See which applications (VS Code, JetBrains, SSH, etc.) users are accessing
- **User Activity**: Identify the most active users and their usage patterns
- **Connection Latency**: Monitor workspace connection performance
- **Template Parameters**: Understand which parameter values users are selecting

## Accessing Template Insights

To view Template Insights:

1. Navigate to the **Templates** page in your Coder dashboard
2. Select a template from the list
3. Click the **Insights** tab

> **Note**: Template Insights requires the `view_insights` permission on the template. Template administrators and organization owners have this permission by default.

## Insights Data

### Active Users Chart

The Active Users chart displays the number of unique users who connected to workspaces from this template over time. You can view data by:

- **Daily intervals**: Shows activity for each day (default for templates less than 5 weeks old)
- **Weekly intervals**: Shows activity aggregated by week (default for templates 5 weeks or older)

Use the date range picker to analyze specific time periods.

### Application Usage

The Application Usage section shows which applications users are connecting through:

#### Built-in Applications

- **VS Code**: Includes VS Code web and desktop connections
- **JetBrains**: JetBrains Gateway connections
- **SSH**: Direct SSH connections to workspaces
- **SFTP**: File transfer connections
- **Web Terminal**: Browser-based terminal sessions

#### Custom Applications

If your template includes custom applications (defined via `coder_app` resources), they will appear in this section with their usage statistics.

For each application, you can see:

- Total seconds of usage during the selected period
- Number of times the application was launched (for custom apps)

### User Activity

The User Activity section lists users who have used workspaces from this template, sorted by total usage time. For each user, you can see:

- Username and avatar
- Total seconds of workspace usage
- Which templates they used (when viewing insights across multiple templates)

This helps identify:

- Power users who may benefit from optimized resources
- Users who might need additional support or training
- Overall adoption of the template

### User Latency

The User Latency section displays connection performance metrics:

- **P50 (Median)**: The median connection latency experienced by users
- **P95**: The 95th percentile latency, indicating worst-case performance for most users

Latency is color-coded for quick assessment:

- **Green**: Good performance (< 150ms)
- **Yellow**: Moderate performance (150-300ms)
- **Red**: Poor performance (> 300ms)

High latency may indicate:

- Network issues between users and workspaces
- Resource constraints on workspace hosts
- Geographic distance between users and infrastructure

### Template Parameters

The Template Parameters section shows which parameter values users are selecting when creating workspaces. This helps you:

- Understand common configuration choices
- Identify unused parameter options
- Optimize default values based on actual usage

For each parameter, you can see:

- Parameter name and type
- Distribution of selected values
- Number of workspaces using each value

## Use Cases

### Capacity Planning

Monitor active user trends to:

- Predict infrastructure capacity needs
- Plan for scaling during peak usage periods
- Identify underutilized templates that could be consolidated

### Template Optimization

Use insights to:

- Remove unused applications or features
- Adjust default parameters based on actual usage patterns
- Optimize resource allocations for common use cases

### User Support

Identify users with:

- High latency connections who may need network troubleshooting
- Low usage who might need onboarding help
- Specific application preferences for targeted support

### ROI and Reporting

Generate reports on:

- Developer productivity through usage metrics
- Template adoption rates
- Infrastructure utilization efficiency

## Permissions

Template Insights respects Coder's RBAC (Role-Based Access Control) system:

- **Template Administrators**: Can view insights for templates they manage
- **Organization Owners**: Can view insights for all templates in their organization
- **Regular Users**: Cannot access Template Insights by default

To grant a user access to Template Insights for a specific template, assign them the `view_insights` permission through [template permissions](./template-permissions.md).

## Data Privacy

Template Insights aggregates usage data while respecting user privacy:

- Individual workspace sessions are aggregated
- User activity shows total usage time, not detailed session logs
- No personally identifiable information beyond usernames is exposed
- Connection latency is measured from agent statistics, not network monitoring

## API Access

Template Insights data is also available via the Coder API. See the [API documentation](../../reference/api/insights.md) for details on:

- `/api/v2/insights/templates` - Template usage metrics
- `/api/v2/insights/user-activity` - User activity data
- `/api/v2/insights/user-latency` - Connection latency metrics

## Troubleshooting

### No Data Displayed

If Template Insights shows no data:

1. **Check permissions**: Ensure you have `view_insights` permission on the template
2. **Verify date range**: Make sure the selected date range includes workspace usage
3. **Confirm workspace activity**: Users must have actively connected to workspaces (workspace creation alone doesn't generate insights)
4. **Wait for data collection**: Insights data is collected from workspace agents and may take a few minutes to appear

### Missing Application Usage

If application usage appears incomplete:

- Ensure workspace agents are up-to-date with the latest Coder version
- Verify that workspaces are running and agents are connected
- Check that applications are configured correctly in the template

### Unexpected Latency Values

If latency metrics seem incorrect:

- Verify that workspace agents can reach the Coder server
- Check for network issues between clients and workspaces
- Ensure DERP (relay) servers are functioning if direct connections fail

## Related Documentation

- [Template Permissions](./template-permissions.md) - Learn about template access control
- [Creating Templates](./creating-templates.md) - Build templates with usage tracking in mind
- [Managing Templates](./managing-templates/change-management.md) - Use insights to inform template updates
