package cli

import "github.com/jason-i-magno/career/internal/model"

// seedCard is one starter prep prompt.
type seedCard struct {
	subject model.Subject
	prompt  string
	ref     string
}

// seedDeck is a starter deck aimed squarely at low-latency C++ and Go backend
// loops. Prompts are phrased as questions you answer out loud, not as facts you
// reread: recognition feels like knowledge and is not.
var seedDeck = []seedCard{
	// --- C++: trading and systems shops probe this hard.
	{model.SubjectCPP, "What is false sharing, how would you detect it, and how do you fix it?", "perf c2c"},
	{model.SubjectCPP, "Walk through std::memory_order relaxed, acquire, release and seq_cst. When is relaxed genuinely sufficient?", ""},
	{model.SubjectCPP, "Why is virtual dispatch costly in a hot loop, and what are the alternatives (CRTP, tagged unions, type erasure)?", ""},
	{model.SubjectCPP, "Design a single-producer single-consumer lock-free ring buffer. Where exactly do the atomics go, and why is it safe without a lock?", ""},
	{model.SubjectCPP, "Why is heap allocation unacceptable on a hot path, and what replaces it? (arena, pool, small-buffer optimisation)", ""},
	{model.SubjectCPP, "Explain move semantics: when does a move actually happen, and when does the compiler silently copy instead?", ""},
	{model.SubjectCPP, "What does noexcept buy you, concretely, in generated code?", ""},
	{model.SubjectCPP, "Compare the cost of unique_ptr and shared_ptr. What does the atomic refcount cost under contention?", ""},
	{model.SubjectCPP, "What is a compiler barrier versus a CPU memory barrier? Give a case where you need each.", ""},
	{model.SubjectCPP, "You have a latency regression in a high-throughput pipeline. What do you measure first, and with what tool?", ""},
	{model.SubjectCPP, "Explain coordinated omission. Why can a naive p99 be badly wrong?", "Gil Tene, How NOT to Measure Latency"},
	{model.SubjectCPP, "When does kernel bypass (DPDK, io_uring, AF_XDP) pay for itself, and what do you give up?", ""},
	{model.SubjectCPP, "Name three undefined behaviours that routinely bite in production C++ and how you guard against them.", ""},
	{model.SubjectCPP, "How do huge pages affect TLB pressure, and when would you enable them?", ""},
	{model.SubjectCPP, "How would you write a branch so the predictor gets it right? When is branchless code worth it?", ""},

	// --- Go: concurrency, the runtime, and the traps that show up in review.
	{model.SubjectGo, "Describe the GMP scheduler. What happens to an M when a goroutine makes a blocking syscall?", ""},
	{model.SubjectGo, "Buffered versus unbuffered channels: what are the exact synchronisation guarantees of each?", ""},
	{model.SubjectGo, "How does context cancellation propagate, and whose job is it to check ctx.Done()?", ""},
	{model.SubjectGo, "When do you reach for a mutex over a channel in Go? Give a case for each.", ""},
	{model.SubjectGo, "Why can an interface holding a nil pointer be non-nil? Show the bug it causes.", ""},
	{model.SubjectGo, "Explain the append aliasing trap: when does append mutate a slice the caller still holds?", ""},
	{model.SubjectGo, "What is escape analysis, and how do you find out whether a value escaped?", "go build -gcflags='-m'"},
	{model.SubjectGo, "Summarise Go's garbage collector. What does GOGC actually control, and when would you change it?", ""},
	{model.SubjectGo, "When are defer arguments evaluated, and what bug does that cause in a loop?", ""},
	{model.SubjectGo, "How do errors.Is and errors.As differ, and when do you wrap with %w versus %v?", ""},
	{model.SubjectGo, "What problem does sync.Pool solve, and why is it wrong to use it as a cache?", ""},
	{model.SubjectGo, "How does the race detector work, and what class of bug does it miss?", ""},

	// --- Distributed systems: the vocabulary backend loops assume you have.
	{model.SubjectDistSys, "State CAP precisely. What is the common misreading?", ""},
	{model.SubjectDistSys, "Why is exactly-once delivery usually a lie, and how do idempotency keys recover the guarantee?", ""},
	{model.SubjectDistSys, "Explain Raft leader election and log replication in two minutes.", "raft.github.io"},
	{model.SubjectDistSys, "What problem does consistent hashing solve, and why do you need virtual nodes?", ""},
	{model.SubjectDistSys, "Give three backpressure strategies and the failure mode each one trades away.", ""},
	{model.SubjectDistSys, "Why does R + W > N give you read-your-writes, and what does it cost?", ""},
	{model.SubjectDistSys, "Compare Lamport timestamps and vector clocks. What can the second do that the first cannot?", ""},
	{model.SubjectDistSys, "Why does exponential backoff need jitter? What happens without it?", ""},
	{model.SubjectDistSys, "Why does nearly every durable system have a write-ahead log?", ""},
	{model.SubjectDistSys, "What is head-of-line blocking, and where does it show up in HTTP/2 versus HTTP/3?", ""},
	{model.SubjectDistSys, "How does a circuit breaker work, and what are its three states?", ""},

	// --- System design: rehearse the shape, not the trivia.
	{model.SubjectSysDes, "Design a distributed rate limiter. Token bucket or sliding window? Where does the state live?", ""},
	{model.SubjectSysDes, "Design a low-latency market-data fan-out: one publisher, thousands of subscribers, microsecond budget.", ""},
	{model.SubjectSysDes, "Design a durable message queue with at-least-once delivery and consumer groups.", ""},
	{model.SubjectSysDes, "Design a metrics ingestion pipeline taking 10M points/sec. What does the storage engine look like?", ""},
	{model.SubjectSysDes, "Design a distributed cache. Cover eviction, invalidation and the thundering herd.", ""},
	{model.SubjectSysDes, "Back-of-envelope: size the fleet for 1M QPS at 20 ms p99. Talk through every assumption.", ""},
	{model.SubjectSysDes, "Design a deployment system that updates hundreds of hosts with no downtime.", ""},
	{model.SubjectSysDes, "How do you shard a database that has outgrown one box? What breaks first?", ""},

	// --- Cloud: what backend job descriptions assume you already have.
	{model.SubjectCloud, "What does a Kubernetes Deployment actually create, and how does a rolling update proceed?", ""},
	{model.SubjectCloud, "Pod requests versus limits: what happens at each threshold for CPU and for memory?", ""},
	{model.SubjectCloud, "Distinguish Service, Ingress and LoadBalancer. Which gives you an external IP, and how?", ""},
	{model.SubjectCloud, "Why does Terraform need remote state and state locking? What goes wrong without them?", ""},
	{model.SubjectCloud, "Explain VPC, subnet, route table and security group. How does a private subnet reach the internet?", ""},
	{model.SubjectCloud, "IAM roles versus users: why should an EC2 instance never hold long-lived keys?", ""},
	{model.SubjectCloud, "Compare blue/green and canary deploys. When is each the right call?", ""},
	{model.SubjectCloud, "Metrics, logs and traces: what question does each answer that the others cannot?", ""},
	{model.SubjectCloud, "How would you move a VM-and-configuration-management deployment to containers plus Kubernetes?", ""},

	// --- DSA: pattern recognition beats problem count.
	{model.SubjectDSA, "What in a problem statement tells you to reach for two pointers or a sliding window?", ""},
	{model.SubjectDSA, "When is a heap the right structure, and what does it cost you versus sorting?", ""},
	{model.SubjectDSA, "What does it mean to binary search on the answer? Give a problem shape that fits.", ""},
	{model.SubjectDSA, "BFS or DFS: what decides it? Which one gives shortest paths on an unweighted graph, and why?", ""},
	{model.SubjectDSA, "How do you recognise a DP problem, and how do you choose the state?", ""},
	{model.SubjectDSA, "What problems does union-find solve that a graph traversal handles badly?", ""},
	{model.SubjectDSA, "What is a monotonic stack for? Name the canonical problem.", ""},
	{model.SubjectDSA, "Given a stream, how do you keep the top k without storing everything?", ""},

	// --- Behaviour: rehearse delivery, not just content.
	{model.SubjectBehavior, "\"Tell me about yourself.\" Ninety seconds, ending on why you are looking now.", ""},
	{model.SubjectBehavior, "\"Why are you leaving your current job?\" Answer without criticising them, and without sounding bored.", ""},
	{model.SubjectBehavior, "If your work is confidential, how do you answer \"describe a hard problem\"? Have the framing ready before you need it.", ""},
	{model.SubjectBehavior, "\"What questions do you have for me?\" Three good ones, tailored to the company.", ""},
	{model.SubjectBehavior, "State your compensation expectations out loud, then stop talking.", ""},
}

// seedStory is a story stub: a title and a hint, left for the user to fill in.
// The set covers every competency in model.AllCompetencies, so a fresh bank
// starts with one placeholder for each question an interviewer will ask.
type seedStory struct {
	title        string
	hint         string
	competencies []model.Competency
}

var seedStories = []seedStory{
	{
		"A project I owned end to end",
		"Lead with what was at stake, then the decision that was yours to make. Interviewers are listening for scope and judgement, not activity.",
		[]model.Competency{model.CompOwnership, model.CompDelivery},
	},
	{
		"The most performance-sensitive thing I have built",
		"Name the constraint before the solution: what was the budget, and what happened if you missed it? Quantify the improvement in relative terms.",
		[]model.Competency{model.CompScale},
	},
	{
		"The hardest bug I have tracked down",
		"Asked in almost every loop. Show your method — how you narrowed it — not just the answer. A story where the first three hypotheses were wrong is better than one where you guessed correctly.",
		[]model.Competency{model.CompDebugging},
	},
	{
		"A time I was wrong",
		"Unavoidable, and the most commonly botched. Pick something with real consequences that you genuinely owned. A fake-humble answer is worse than none.",
		[]model.Competency{model.CompFailure},
	},
	{
		"A disagreement with a colleague",
		"Interviewers want to see you disagree technically without making it personal, and change your mind when the evidence says so. The ending matters more than the argument.",
		[]model.Competency{model.CompConflict},
	},
	{
		"Bringing a teammate up to speed",
		"Name what they could not do before and can do now. Mentoring stories fail when they describe your generosity instead of their growth.",
		[]model.Competency{model.CompMentoring},
	},
	{
		"A project where the requirements were unclear",
		"An ambiguity story is really a risk story. What did you not know at the start, and how did you de-risk it before committing?",
		[]model.Competency{model.CompAmbiguity},
	},
	{
		"A change I drove without any authority",
		"How did you build the case, and who did you have to convince? This is the story that separates senior from mid-level.",
		[]model.Competency{model.CompInfluence},
	},
	{
		"Work that crossed a team boundary",
		"Cross-team stories are about communication overhead. What broke at the seam, and what did you put in place so it stopped breaking?",
		[]model.Competency{model.CompCollab},
	},
}
