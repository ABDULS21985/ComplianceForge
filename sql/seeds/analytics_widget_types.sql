BEGIN;

INSERT INTO analytics_widget_types (id, widget_type, name, description, available_metrics, default_config, min_width, min_height) VALUES
(gen_random_uuid(), 'line_chart', 'Line Chart', 'Time series line chart for tracking trends over time', '{compliance_score,risk_count,incident_count,finding_count,attestation_rate,vendor_risk_score}', '{"period": "12m", "granularity": "monthly"}', 4, 3),
(gen_random_uuid(), 'bar_chart', 'Bar Chart', 'Vertical or horizontal bar chart for comparisons', '{compliance_score_by_framework,risks_by_category,incidents_by_severity,findings_by_status,vendors_by_risk_tier}', '{"orientation": "vertical"}', 4, 3),
(gen_random_uuid(), 'donut_chart', 'Donut Chart', 'Donut/pie chart for distribution breakdowns', '{risks_by_level,controls_by_status,incidents_by_status,policies_by_status,findings_by_severity}', '{"show_legend": true}', 3, 3),
(gen_random_uuid(), 'kpi_card', 'KPI Card', 'Single metric display with trend indicator and comparison', '{compliance_score,total_risks,open_incidents,open_findings,policies_due,high_risk_vendors,attestation_rate,treatment_completion}', '{"comparison": "previous_month"}', 2, 2),
(gen_random_uuid(), 'heatmap', 'Risk Heatmap', 'Interactive 5x5 risk likelihood vs impact heatmap', '{risk_heatmap_inherent,risk_heatmap_residual}', '{"mode": "residual", "size": 5}', 4, 4),
(gen_random_uuid(), 'radar', 'Radar Chart', 'Multi-axis radar for framework comparison or maturity assessment', '{framework_compliance_scores,framework_maturity_levels}', '{}', 4, 4),
(gen_random_uuid(), 'table', 'Data Table', 'Sortable, filterable data table', '{top_risks,recent_incidents,overdue_findings,vendor_assessments,gap_analysis}', '{"rows": 10, "sortable": true}', 6, 4),
(gen_random_uuid(), 'gauge', 'Gauge Meter', 'Semicircular gauge showing a value against a target', '{compliance_score,breach_probability,treatment_completion,attestation_rate}', '{"target": 80, "zones": [{"min": 0, "max": 60, "color": "red"}, {"min": 60, "max": 80, "color": "amber"}, {"min": 80, "max": 100, "color": "green"}]}', 2, 2),
(gen_random_uuid(), 'sparkline', 'Sparkline', 'Compact inline trend visualization', '{compliance_score,risk_count,incident_count}', '{"period": "30d"}', 2, 1),
(gen_random_uuid(), 'trend_arrow', 'Trend Arrow', 'Simple up/down/flat trend indicator with percentage change', '{compliance_score_change,risk_score_change,incident_rate_change}', '{"period": "30d"}', 1, 1),
(gen_random_uuid(), 'map', 'Geographic Map', 'Map showing data distribution by region/country', '{vendors_by_country,incidents_by_region,regulatory_changes_by_jurisdiction}', '{"region": "europe"}', 6, 4);

COMMIT;
