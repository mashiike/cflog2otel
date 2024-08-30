package cflog2otel

type AggregationType int

//go:generate go install github.com/dmarkham/enumer@latest
//go:generate enumer -type=AggregationType -trimprefix=AggregationType -json -text
const (
	AggregationTypeCounter AggregationType = iota
)
