# Bot protocol

A bot is a program that connects to the King server over WebSocket, receives one request per decision, and sends back an answer. You can write it in any language. The messages are JSON, and every message is described in [`schema/king.schema.json`](schema/king.schema.json). The game rules are in [RULES.md](RULES.md).

## Connecting

Connect to the URL the match organizer gives you:

- Same machine or LAN: `ws://<host>:8080/ws`
- Over the internet (Cloudflare tunnel): `wss://<random>.trycloudflare.com/ws`

Every message is one WebSocket **text** message containing one JSON object. Every object has a `"type"` field.

## Handshake

The first message must be `hello`, sent within 10 seconds of connecting:

```json
{"type": "hello", "name": "alice"}
```

`name` identifies your bot for the whole match (1–32 bytes). The server answers with `welcome`:

```json
{"type": "welcome", "name": "alice"}
```

or with `error` and closes the connection:

```json
{"type": "error", "message": "name taken"}
```

The possible errors are:
- `name taken`: a bot with this name is connected.
- `match is full`: 4 bots are already registered.
- `match already started`: unknown names can't join once the match has begun.
- `first message must be hello`

Bots get seats in the order they connect. **The match starts automatically once 4 bots with distinct names are connected.**

## Requests and responses

For each decision, the server sends a `request`. Each request has a unique `id`, and you answer with a `response` carrying the same `id`. There are two kinds of request.

### `choose_contract`

You're the declarer of this hand. `legal_contracts` lists every contract you may declare. Here is an actual first request of a game:

```json
{"type":"request","id":"17-0","game_id":17,"kind":"choose_contract","state":{"game_id":17,"seat":3,"players":["alice","bob","cenk","deniz"],"hand_number":1,"declarer":3,"cards":[{"rank":3,"suit":"C"},{"rank":9,"suit":"C"},{"rank":11,"suit":"C"},{"rank":2,"suit":"D"},{"rank":6,"suit":"D"},{"rank":11,"suit":"D"},{"rank":12,"suit":"D"},{"rank":3,"suit":"H"},{"rank":4,"suit":"H"},{"rank":12,"suit":"H"},{"rank":3,"suit":"S"},{"rank":7,"suit":"S"},{"rank":11,"suit":"S"}],"tricks":[],"current_trick":{"leader":3,"cards":[],"winner":-1},"history":[],"totals":[0,0,0,0],"negative_counts":{"no_hearts":0,"no_king_of_hearts":0,"no_kings_jacks":0,"no_last_two":0,"no_queens":0,"no_tricks":0},"quotas":[{"trump":2,"negative":3},{"trump":2,"negative":3},{"trump":2,"negative":3},{"trump":2,"negative":3}]},"legal_contracts":[{"kind":"no_tricks"},{"kind":"no_hearts"},{"kind":"no_queens"},{"kind":"no_kings_jacks"},{"kind":"no_last_two"},{"kind":"no_king_of_hearts"},{"kind":"trump","suit":"C"},{"kind":"trump","suit":"D"},{"kind":"trump","suit":"H"},{"kind":"trump","suit":"S"}]}
```

Answer with one of them:

```json
{"type": "response", "id": "17-0", "contract": {"kind": "trump", "suit": "S"}}
```

### `play_card`

It's your turn to play a card. `legal_cards` lists every card you may play. In this example, spades are trump, seat 3 led 2♠, seat 0 followed with 5♠, and you (seat 1) must follow suit with your only spade:

```json
{
  "type": "request", "id": "17-31", "game_id": 17, "kind": "play_card",
  "state": {
    "game_id": 17, "seat": 1, "players": ["alice", "bob", "cenk", "deniz"],
    "hand_number": 2, "declarer": 0,
    "contract": {"kind": "trump", "suit": "S"},
    "cards": [
      {"rank": 8, "suit": "C"}, {"rank": 12, "suit": "C"}, {"rank": 14, "suit": "C"},
      {"rank": 4, "suit": "D"}, {"rank": 7, "suit": "D"}, {"rank": 8, "suit": "D"}, {"rank": 9, "suit": "D"},
      {"rank": 4, "suit": "H"}, {"rank": 5, "suit": "H"}, {"rank": 10, "suit": "H"}, {"rank": 11, "suit": "H"},
      {"rank": 11, "suit": "S"}
    ],
    "tricks": [
      {"leader": 0, "cards": [{"rank": 2, "suit": "C"}, {"rank": 6, "suit": "C"}, {"rank": 4, "suit": "C"}, {"rank": 9, "suit": "C"}], "winner": 3}
    ],
    "current_trick": {"leader": 3, "cards": [{"rank": 2, "suit": "S"}, {"rank": 5, "suit": "S"}], "winner": -1},
    "history": [
      {"hand_number": 1, "declarer": 3, "contract": {"kind": "no_queens"}, "scores": [0, -100, -200, -100]}
    ],
    "totals": [0, -100, -200, -100],
    "negative_counts": {"no_hearts": 0, "no_king_of_hearts": 0, "no_kings_jacks": 0, "no_last_two": 0, "no_queens": 1, "no_tricks": 0},
    "quotas": [{"trump": 1, "negative": 3}, {"trump": 2, "negative": 3}, {"trump": 2, "negative": 3}, {"trump": 2, "negative": 2}]
  },
  "legal_cards": [{"rank": 11, "suit": "S"}]
}
```

```json
{"type": "response", "id": "17-31", "card": {"rank": 11, "suit": "S"}}
```

### Cards and contracts

- **Card:** `{"rank": 12, "suit": "S"}`. Rank is 2–14 (J = 11, Q = 12, K = 13, A = 14). Suit is `C`, `D`, `H` or `S`.
- **Contract:** `{"kind": "<kind>"}` for the negatives: `no_tricks`, `no_hearts`, `no_queens`, `no_kings_jacks`, `no_last_two`, `no_king_of_hearts`. Trump is `{"kind": "trump", "suit": "S"}`.

**Always answer with an entry from `legal_cards` / `legal_contracts`.** The server has already applied every rule. You never have to work out legality yourself, though you may want to when planning ahead.

### The `state` object

`state` is complete. You don't need to remember anything between requests.

| Field | Meaning |
|---|---|
| `game_id` | Game number in the match, 0–99. |
| `seat` | Your seat, 0–3. Seats change between games (seat rotation), so always read this. |
| `players` | Bot names by seat. |
| `hand_number` | 1–20. |
| `declarer` | Seat that chose this hand's contract. |
| `contract` | This hand's contract. Absent in `choose_contract` requests. |
| `cards` | Your cards right now. |
| `tricks` | Completed tricks of this hand, in order. |
| `current_trick` | The trick in progress. |
| `history` | Earlier hands of this game: declarer, contract and the score by seat. |
| `totals` | Game scores by seat, *not* counting the current hand. |
| `negative_counts` | How many times each negative contract has been declared this game (max 2). |
| `quotas` | By seat: trump and negative declarations still owed (each player declares 2 trump + 3 negative). |

A **trick** is `{"leader": seat, "cards": [...], "winner": seat}`. `cards` are in play order, starting with the leader. Play is **counter-clockwise**: `cards[i]` was played by seat `(leader + i) % 4`. `winner` is `-1` while the trick is incomplete. Each hand is a new deal, so cards from earlier hands don't matter. That's why `history` holds only scores.

## Timeouts and faults

- You have **2 seconds** per decision by default. The organizer can change this with `-timeout`.
- If you're late, or your answer isn't one of the legal options (including malformed JSON with a valid `id`), the server plays your **lowest legal** option for you and records a **strike**:
  - For a card, lowest means the lowest rank, with ties broken by ♣ ♦ ♥ ♠.
  - For a contract, it's the first entry of `legal_contracts`.
- An answer that arrives after the timeout is ignored.
- Strikes are shown in the standings. They don't cost points directly, but the fallback move is usually a bad one.

## Disconnects: your bot must reconnect

If your connection drops, **the match pauses**. Nothing times out while you're gone, and you get no strike. When you connect again and send `hello` with the **same name**, you get your seat back and the server re-sends the pending request.

So a bot should loop forever: connect → hello → answer requests → on error, wait and reconnect. Use a short backoff (e.g. 100 ms, doubling, capped at 5 s). If you get `name taken` right after a drop, the server hasn't noticed that your old connection is dead yet. Keep retrying.

The server pings every 15 s. Standard WebSocket libraries answer pings automatically.

## Several requests at once

If the organizer runs with `-parallel N`, up to N games run at the same time, and you may have several requests outstanding. Each one carries its own `game_id`, `id` and full `state`. Answer each by `id`, in any order. If your bot handles each request independently (it should, since `state` is complete), this just works. Make sure your writes to the socket are safe from several threads, or answer one request at a time.

## Match end

When the match is over, the server sends the final standings and closes the connection:

```json
{"type": "match_end", "standings": [{"rank": 1, "name": "alice", "placement_points": 171.5, "total_points": 6420, "games_won": 31, "strikes": 0}]}
```

The winner is the bot with the most **placement points** (3/2/1/0 per game, ties split).

## A minimal bot (Python)

```python
# pip install websockets
import asyncio, json, random, websockets

async def main(url, name):
    while True:
        try:
            async with websockets.connect(url) as ws:
                await ws.send(json.dumps({"type": "hello", "name": name}))
                async for raw in ws:
                    msg = json.loads(raw)
                    if msg["type"] == "match_end":
                        return
                    if msg["type"] != "request":
                        continue
                    resp = {"type": "response", "id": msg["id"]}
                    if msg["kind"] == "choose_contract":
                        resp["contract"] = random.choice(msg["legal_contracts"])
                    else:
                        resp["card"] = random.choice(msg["legal_cards"])
                    await ws.send(json.dumps(resp))
        except (OSError, websockets.ConnectionClosed):
            await asyncio.sleep(1)  # reconnect

asyncio.run(main("ws://127.0.0.1:8080/ws", "alice"))
```

There is a Go example in [`cmd/examplebot`](cmd/examplebot/main.go).

## Generating types

Use [quicktype](https://quicktype.io) to generate typed classes from the schema. Point it at each message definition, not at the whole file, so you get one class per message:

```sh
S=schema/king.schema.json
npx quicktype --src-lang schema --lang python \
  "$S#/\$defs/Request" "$S#/\$defs/Response" "$S#/\$defs/Hello" \
  "$S#/\$defs/Welcome" "$S#/\$defs/Error" "$S#/\$defs/MatchEnd" \
  -o king_types.py
```

Replace `--lang python` with `typescript`, `rust`, `csharp`, `java`, `kotlin`, `swift`, and so on. With Python older than 3.10, add `--python-version 3.7`. `king schema` prints the schema, so you can also generate types from a server binary.

## Testing your bot

Each of these runs the server on your laptop, and you connect your bot to `ws://127.0.0.1:8080/ws`.

- `king validate`: sends your bot 24 tricky positions (forced discards, forced trumps, quotas, the last trick, …) and reports for each whether your answer was legal. It exits non-zero on any failure.
- `king practice -bots 1 -games 20`: a real match against 3 house bots (random, low, low).
- `king practice -bots 2`: two of your bots, plus 2 house bots.
