# Loopism

## An ideological manifesto: *Loopism*

We reject the cult of the pristine first draft.
We reject the fantasy that the best software emerges fully formed from solitary genius.
We reject “perfect” as procrastination wearing a lab coat.

We choose the loop.

### 1) The loop is the unit of progress

A plan is not progress. A prompt is not progress. A pull request is not progress.
Progress is a **closed cycle**: *intend → run → observe → correct → repeat.*

### 2) Compute is the unfair advantage—and loops are how you spend it

Sutton’s Bitter Lesson is not just history; it’s a warning label: the systems that win are the ones that **keep getting better as you scale compute**. 
So we stop worshipping fragile cleverness and start building **general, repeatable machinery** that improves when you run it more.

### 3) The agent is replaceable; the loop is compounding

Models will change. Vendors will change. Tooling will change.
But a well-designed loop outlives any single model release—because it’s a *method* that automatically benefits when the underlying engine improves. Huntley’s “Ralph is a Bash loop” framing is radical precisely because it’s boring: the power is in repetition.

### 4) Deterministic failure is a gift

If the agent fails in consistent ways, we can build consistent defenses:
tests, linters, type checks, sandboxing, permissions, diffs, rollbacks, budgets.
We don’t demand perfection from the agent—we demand **legibility from the process**.

### 5) We build “eventual consistency” systems

We accept that iteration converges.
We accept that early outputs are wrong.
We accept that “working” is not a state, it’s a direction.

### 6) Orchestration is the new craftsmanship

The craft is no longer typing code; it’s **designing the rails**:

* what the agent can touch
* what it must prove (tests, checks)
* how it reports progress
* when it escalates to a human
* how it recovers from mistakes

### 7) Self-healing is not magic—it’s instrumentation plus retries

“Self-healing” doesn’t mean “never breaks.”
It means: breakage becomes a **signal**, signals trigger **work**, work is rerun until stable.
Huntley describes pipelines where failures feed back and get repaired through feedback loops.
That’s not mysticism. That’s engineering.

### 8) We don’t ship opinions; we ship feedback loops

If you can’t test it, you can’t automate it.
If you can’t observe it, you can’t trust it.
If you can’t bound it, you can’t scale it.

### 9) “Iteration trumps perfection” becomes policy

* default to small steps
* prefer reversible changes
* make success measurable
* make failure cheap
* rerun relentlessly

### 10) The new role of the human: choose objectives, define constraints, audit outcomes

Humans are not replaced by agents. Humans are **promoted**:
from implementers to operators, from authors to governors, from coders to designers of systems that code.

### 11) Our slogan

**Run the loop. Watch the loop. Tune the loop.**
Everything else is theater.

If you want, I can also translate this into a practical “Loop Spec” template (inputs, constraints, stop conditions, evals, safety rails) you can drop into any repo and use with whichever agent CLI you like.


## REFERECNCES:

> **The Loop Is the Product**
> Geoffrey Huntley’s “Ralph Wiggum” technique frames modern software creation as a *compute-driven iteration loop*: run a coding agent in a simple loop (in the pure form, literally a Bash `while` loop), let it attempt the task, observe what changed, and run it again until the work is done. Huntley’s emphasis is that the method is “deterministically bad” but predictable—so you can *tune the loop* (prompts + guardrails + tests) until it converges. ([Geoffrey Huntley][1])

> Rich Sutton’s “Bitter Lesson” argues that, across AI history, the winners are **general methods that scale with compute**—especially *search and learning*—while clever, human-shaped handcrafting tends to help briefly but loses long-term. ([Rich Sutton][2])

[1]: https://ghuntley.com/ralph/ "Ralph Wiggum as a \"software engineer\""
[2]: http://www.incompleteideas.net/IncIdeas/BitterLesson.html "The Bitter Lesson"
