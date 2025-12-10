package stages

import "context"

type Stage[In, Out any] func(context.Context, In) Out

type Stages[In, Out any] []Stage[In, Out]

// Next sends the ReqRes to the next stage.
func (s *Stages[In, Out]) Next(ctx context.Context, in In) Out {
	nextStage := (*s)[0]
	*s = (*s)[1:]
	return nextStage(ctx, in)
}

func (s Stages[In, Out]) Copy() Stages[In, Out] {
	stagesCopy := make([]Stage[In, Out], len(s))
	copy(stagesCopy, s)
	return stagesCopy
}
