package cflog2otel

import (
	"context"
	"log/slog"
	"time"

	"github.com/samber/oops"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/instrumentation"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.opentelemetry.io/otel/sdk/resource"
)

func Aggregate(ctx context.Context, cfg *Config, celVariables *CELVariables, logs []CELVariablesLog) ([]*metricdata.ResourceMetrics, error) {
	resourceMetrics := make([]*metricdata.ResourceMetrics, 0)
	for _, l := range logs {
		celVariables.SetLogLine(l)
		attrs, err := ToAttributes(ctx, cfg.ResourceAttributes, celVariables)
		if err != nil {
			return nil, oops.Wrapf(err, "failed to convert attributes")
		}
		var found bool
		attrSet := attribute.NewSet(attrs...)
		var target *metricdata.ResourceMetrics
		for i, r := range resourceMetrics {
			set := attribute.NewSet(r.Resource.Attributes()...)
			if !set.Equals(&attrSet) {
				continue
			}
			target = resourceMetrics[i]
			found = true
		}
		if !found {
			target = &metricdata.ResourceMetrics{
				Resource: resource.NewSchemaless(attrs...),
				ScopeMetrics: []metricdata.ScopeMetrics{
					{
						Scope: instrumentation.Scope{
							Name:      cfg.Scope.Name,
							Version:   cfg.Scope.Version,
							SchemaURL: cfg.Scope.SchemaURL,
						},
						Metrics: make([]metricdata.Metrics, 0, len(cfg.Metrics)),
					},
				},
			}
			resourceMetrics = append(resourceMetrics, target)
		}
		for _, mcfg := range cfg.Metrics {
			var found bool
			var metricsIndex int
			for i, metric := range target.ScopeMetrics[0].Metrics {
				if mcfg.Name != metric.Name {
					continue
				}
				metricsIndex = i
				found = true
				break
			}
			if !found {
				target.ScopeMetrics[0].Metrics = append(target.ScopeMetrics[0].Metrics, metricdata.Metrics{
					Name:        mcfg.Name,
					Description: mcfg.Description,
					Unit:        mcfg.Unit,
				})
				metricsIndex = len(target.ScopeMetrics[0].Metrics) - 1
			}
			target.ScopeMetrics[0].Metrics[metricsIndex], err = aggregateMetric(ctx, target.ScopeMetrics[0].Metrics[metricsIndex], mcfg, celVariables)
			if err != nil {
				return nil, oops.Wrapf(err, "failed to aggregate metric %q", mcfg.Name)
			}
		}
	}
	resp := make([]*metricdata.ResourceMetrics, 0, len(resourceMetrics))
	for _, r := range resourceMetrics {
		if r == nil {
			continue
		}
		metrics := make([]metricdata.Metrics, 0, len(r.ScopeMetrics[0].Metrics))
		for i, m := range r.ScopeMetrics[0].Metrics {
			if LenDataPoints(m.Data) == 0 {
				continue
			}
			metrics = append(metrics, r.ScopeMetrics[0].Metrics[i])
		}
		if len(metrics) == 0 {
			continue
		}
		r.ScopeMetrics[0].Metrics = metrics
		resp = append(resp, r)
	}
	return resp, nil
}

func LenDataPoints(data metricdata.Aggregation) int {
	if data == nil {
		return 0
	}
	switch data := data.(type) {
	case metricdata.Sum[int64]:
		return len(data.DataPoints)
	case metricdata.Sum[float64]:
		return len(data.DataPoints)
	case metricdata.Histogram[float64]:
		return len(data.DataPoints)
	default:
		return 0
	}
}

func aggregateMetric(ctx context.Context, metrics metricdata.Metrics, config MetricsConfig, vars *CELVariables) (metricdata.Metrics, error) {
	if config.Filter != nil {
		isTarget, err := config.Filter.Eval(ctx, vars)
		if err != nil {
			return metrics, oops.Wrapf(err, "failed to evaluate filter")
		}
		if !isTarget {
			slog.DebugContext(ctx, "not a target log, skipping")
			return metrics, nil
		}
	}
	switch config.Type {
	case AggregationTypeCount:
		return aggregateForCountMetric(ctx, metrics, config, vars)
	case AggregationTypeSum:
		return aggregateForSumMetric(ctx, metrics, config, vars)
	case AggregationTypeHistogram:
		return aggregateForHistogramMetric(ctx, metrics, config, vars)
	default:
		return metricdata.Metrics{}, oops.Errorf("unsupported aggregation type %q", config.Type)
	}
}

func getAggregateAxis(ctx context.Context, config MetricsConfig, vars *CELVariables) (time.Time, time.Time, attribute.Set, error) {
	t := vars.Log.Timestamp.Truncate(time.Minute)
	attrs, err := ToAttributes(ctx, config.Attributes, vars)
	if err != nil {
		return time.Time{}, time.Time{}, attribute.Set{}, oops.Wrapf(err, "failed to convert attributes")
	}
	attrSet := attribute.NewSet(attrs...)
	return t, t.Add(time.Minute), attrSet, nil
}

func aggregateForCountMetric(ctx context.Context, metrics metricdata.Metrics, config MetricsConfig, vars *CELVariables) (metricdata.Metrics, error) {
	if metrics.Data == nil {
		temporality := metricdata.DeltaTemporality
		if config.IsCumulative {
			temporality = metricdata.CumulativeTemporality
		}
		metrics.Data = metricdata.Sum[int64]{
			DataPoints:  make([]metricdata.DataPoint[int64], 0),
			Temporality: temporality,
			IsMonotonic: true,
		}
	}
	data, ok := metrics.Data.(metricdata.Sum[int64])
	if !ok {
		return metrics, oops.Errorf("unsupported data type for counter")
	}
	startTime, t, attrSet, err := getAggregateAxis(ctx, config, vars)
	if err != nil {
		return metrics, oops.Wrapf(err, "failed to get aggregate axis")
	}
	var found bool
	for i, dp := range data.DataPoints {
		if !dp.Time.Equal(t) {
			continue
		}
		if !dp.Attributes.Equals(&attrSet) {
			continue
		}
		data.DataPoints[i].Value++
		found = true
		break
	}
	if !found {
		data.DataPoints = append(data.DataPoints, metricdata.DataPoint[int64]{
			StartTime:  startTime,
			Time:       t,
			Value:      1,
			Attributes: attrSet,
		})
	}
	metrics.Data = data
	return metrics, nil
}

func aggregateForSumMetric(ctx context.Context, metrics metricdata.Metrics, config MetricsConfig, vars *CELVariables) (metricdata.Metrics, error) {
	if metrics.Data == nil {
		temporality := metricdata.DeltaTemporality
		if config.IsCumulative {
			temporality = metricdata.CumulativeTemporality
		}
		metrics.Data = metricdata.Sum[float64]{
			DataPoints:  make([]metricdata.DataPoint[float64], 0),
			Temporality: temporality,
			IsMonotonic: config.IsMonotonic,
		}
	}
	data, ok := metrics.Data.(metricdata.Sum[float64])
	if !ok {
		return metrics, oops.Errorf("unsupported data type for counter")
	}
	startTime, t, attrSet, err := getAggregateAxis(ctx, config, vars)
	if err != nil {
		return metrics, oops.Wrapf(err, "failed to get aggregate axis")
	}
	value, err := config.Value.Eval(ctx, vars)
	if err != nil {
		return metrics, oops.Wrapf(err, "failed to evaluate value")
	}
	var found bool
	for i, dp := range data.DataPoints {
		if !dp.Time.Equal(t) {
			continue
		}
		if !dp.Attributes.Equals(&attrSet) {
			continue
		}

		data.DataPoints[i].Value += value
		found = true
		break
	}
	if !found {
		data.DataPoints = append(data.DataPoints, metricdata.DataPoint[float64]{
			StartTime:  startTime,
			Time:       t,
			Value:      value,
			Attributes: attrSet,
		})
	}
	metrics.Data = data
	return metrics, nil
}

func aggregateForHistogramMetric(ctx context.Context, metrics metricdata.Metrics, config MetricsConfig, vars *CELVariables) (metricdata.Metrics, error) {
	if metrics.Data == nil {
		temporality := metricdata.DeltaTemporality
		if config.IsCumulative {
			temporality = metricdata.CumulativeTemporality
		}
		metrics.Data = metricdata.Histogram[float64]{
			DataPoints:  make([]metricdata.HistogramDataPoint[float64], 0),
			Temporality: temporality,
		}
	}
	data, ok := metrics.Data.(metricdata.Histogram[float64])
	if !ok {
		return metrics, oops.Errorf("unsupported data type for histogram")
	}
	startTime, t, attrSet, err := getAggregateAxis(ctx, config, vars)
	if err != nil {
		return metrics, oops.Wrapf(err, "failed to get aggregate axis")
	}
	value, err := config.Value.Eval(ctx, vars)
	if err != nil {
		return metrics, oops.Wrapf(err, "failed to evaluate value")
	}
	var targetDPIndex int
	var found bool
	for i, dp := range data.DataPoints {
		if !dp.Time.Equal(t) {
			continue
		}
		if !dp.Attributes.Equals(&attrSet) {
			continue
		}
		targetDPIndex = i
		found = true
		break
	}
	if !found {
		dp := metricdata.HistogramDataPoint[float64]{
			StartTime:  startTime,
			Time:       t,
			Count:      0,
			Sum:        0,
			Attributes: attrSet,
		}
		if config.Boundaries != nil {
			dp.Bounds = make([]float64, len(config.Boundaries))
			copy(dp.Bounds, config.Boundaries)
			dp.BucketCounts = make([]uint64, len(config.Boundaries)+1)
		}
		data.DataPoints = append(data.DataPoints, dp)
		targetDPIndex = len(data.DataPoints) - 1
	}
	data.DataPoints[targetDPIndex] = AppendValueToHistogramDataPoint(value, data.DataPoints[targetDPIndex], config.NoMinMax)
	metrics.Data = data
	return metrics, nil
}

func AppendValueToHistogramDataPoint[N int64 | float64](value N, dp metricdata.HistogramDataPoint[N], noMinMax bool) metricdata.HistogramDataPoint[N] {
	dp.Count++
	dp.Sum += value
	if !noMinMax {
		if cmin, defined := dp.Min.Value(); !defined || value < cmin {
			dp.Min = metricdata.NewExtrema(value)
		}
		if cmax, defined := dp.Max.Value(); !defined || value > cmax {
			dp.Max = metricdata.NewExtrema(value)
		}
	}
	if dp.Bounds == nil {
		return dp
	}
	// for example: bounds=[0, 5, 10, 15]
	// bucketCounts means (-inf, 0], (0, 5], (5, 10], (10, 15], (15, +inf)
	// so, the value 0 is in the second bucket, the value 5 is in the third bucket
	v := float64(value)
	for i, b := range dp.Bounds {
		if v < b {
			dp.BucketCounts[i]++
			return dp
		}
	}
	dp.BucketCounts[len(dp.Bounds)]++
	return dp
}
