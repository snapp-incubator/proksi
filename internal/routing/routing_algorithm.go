package routing

// R is a global variable for using RoutingAlgorithm interface.
var R RoutingAlgorithm

// RoutingAlgorithm is an interface for routing algorithm.
type RoutingAlgorithm interface {
	IsInBucket() bool
}

// routingBucket is a struct for routing algorithms. it's contain a weight of bucket.
type routingBucket struct {
	weight    int
	oldWeight int
	newWeight int
}

// NewRoutingBucket is a factory method for routingBucket.
func NewRoutingBucket(weight int) RoutingAlgorithm {
	if weight < 0 || weight > 100 {
		panic("weight must be between 0 and 100")
	}

	return &routingBucket{
		weight:    weight,
		newWeight: weight,
		oldWeight: 100 - weight,
	}
}

// IsInBucket is a method for checking in bucket or not. it's return true if newWeight greater than oldWeight.
func (b *routingBucket) IsInBucket() bool {
	isInBucket := b.newWeight > b.oldWeight
	b.updateWeight(isInBucket)
	return isInBucket
}

// updateWeight is a method for updating weight of each bucket.
func (b *routingBucket) updateWeight(isInBucket bool) {
	if isInBucket {
		b.newWeight -= 100 - b.weight
		b.oldWeight += 100 - b.weight
	} else {
		b.newWeight += b.weight
		b.oldWeight -= b.weight
	}

	if b.newWeight == 100 || b.oldWeight == 100 {
		b.newWeight = b.weight
		b.oldWeight = 100 - b.weight
	}
}
