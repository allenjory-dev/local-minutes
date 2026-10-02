# Synthetic summary fixtures

Every transcript here is invented for tests. None comes from a real recording,
meeting or person. Technical numbers and clause references are deliberately
unverified test material, not code guidance.

Each file holds a stored-format transcript (`interfaces.TranscriptResult`
JSON) and one or more `model_outputs`: mocked responses that reproduce
behaviour observed during local qualification (suggestions promoted to
actions, wrong speakers, paraphrases in quotation marks, superseded deadlines,
obeyed injected instructions) next to well-formed answers. Tests assert what
the application does with each response; they never depend on a live model.
