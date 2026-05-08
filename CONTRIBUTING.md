
## About Coding Agent Instructions

コーディングエージェントを利用する場合は、`ai-instructions.md`のシンボリックリンクを各エージェントの共通の指示として使ってください。

```sh
# Claude
ln -s ./ai-instructions.md ./CLAUDE.md

# Codex
ln -s ./ai-instructions.md ./AGENTS.md

# GitHub Copilot
mkdir .github
ln -s ../ai-instructions.md ./.github/copilot-instructions.md
```