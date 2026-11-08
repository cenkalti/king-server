# king-server

An orchestrator for bot matches of Turkish King (Rıfkı). Each player runs their own bot, which connects to the server over WebSocket. The server deals the cards, enforces the rules and runs a 100-game match with duplicate deals. The bot with the most placement points wins.

- [RULES.md](RULES.md): the exact rules the server enforces.
- [PROTOCOL.md](PROTOCOL.md): how a bot talks to the server.
- `schema/king.schema.json`: JSON Schema for every message. Generate types for your language with `quicktype`.
