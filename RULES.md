# Rules

The server enforces Turkish King (Rıfkı) exactly as described at <https://www.pagat.com/compendium/rifki.html>. When this document and the server disagree, the server has a bug.

## Table

- 4 players, each playing for themselves. 52 cards, 13 per player. Ranks from high to low: A K Q J 10 9 … 2.
- **Play goes counter-clockwise.** Seats are numbered 0–3. The player after seat `s` is seat `(s + 1) mod 4`.
- A game is **20 hands**. Each hand is dealt fresh. Who deals has no effect, because the server deals.

## Declaring

- Each hand has a **declarer**, who chooses that hand's contract.
- The declarer of hand 1 is whoever holds the **2♦** in hand 1. The declarer role then moves to the next seat (counter-clockwise) every hand, so each player declares exactly **5 times**.
- Of their 5 declarations, each player must choose **exactly 2 trump contracts and 3 negative contracts**.
- Each of the 6 negative contracts can be declared **at most twice per game**, by anyone.
- The server sends the declarer only the contracts that are still legal for them.

## Play

- **The declarer leads the first trick.** The winner of each trick leads the next one.
- In every contract, **you must follow suit if you can**.
- A trick is won by the highest card of the suit led, or in a trump contract by the highest trump in the trick, if there is one.
- Apart from the specific rules below, you never have to beat the cards already in the trick.

## Trump contracts: `trump`

- The declarer names a trump suit: `C`, `D`, `H` or `S`. Notrump isn't allowed.
- If you can't follow suit, **you must play a trump if you have one**, even if a higher trump has already been played (an "underruff").
- If you can't follow suit and have no trumps, play any card.
- You may lead trump at any time.
- **+50 per trick won** (650 per hand in total).

*Example.* Spades are trump. A♥ is led. You have no hearts. The previous player trumped with 9♠. You hold 3♠, Q♠ and 7♦. **Legal: 3♠ or Q♠.** You must play a spade, but not necessarily the higher one. 7♦ is illegal.

## Negative contracts

There are no trumps in negative contracts. There are two kinds:

| Wire name | Turkish | Penalty | Hand total |
|---|---|---|---|
| `no_tricks` | El almaz | −50 per trick | −650 |
| `no_hearts` | Kupa almaz | −30 per heart | −390 |
| `no_queens` | Kız almaz | −100 per queen | −400 |
| `no_kings_jacks` | Erkek almaz | −60 per king or jack | −480 |
| `no_last_two` | Son iki | −180 for each of tricks 12 and 13 | −360 |
| `no_king_of_hearts` | Rıfkı | −320 for K♥ | −320 |

The **card-based** contracts are `no_hearts`, `no_queens`, `no_kings_jacks` and `no_king_of_hearts`. Their "penalty cards" are the cards listed above. The **trick-based** contracts are `no_tricks` and `no_last_two`, which have no penalty cards.

Penalty points go to whoever wins the trick that contains the penalty card (or the penalized trick).

### Rule N1: Forced penalty play (card-based contracts only)

If you're following suit and hold a penalty card of that suit, and **a higher card of that suit has already been played to this trick**, you must play a penalty card of that suit. You can't hold it back. If you hold several penalty cards of that suit, you may play any of them, even one that wins the trick.

*Example (`no_queens`).* 5♦ is led, and the next player plays K♦. You hold Q♦ and 3♦. **Legal: Q♦ only.** The queen can't win the trick, so you must give it to the K♦ player.

*Example (`no_queens`).* 5♦ is led, and the next player plays 9♦. You hold Q♦ and 3♦. **Legal: Q♦ or 3♦.** Nothing higher than the queen has been played, so the rule doesn't apply.

*Example (`no_kings_jacks`).* 4♣ is led, and Q♣ has been played. You hold K♣, J♣ and 2♣. **Legal: K♣ or J♣.** Q♣ is higher than your J♣, so you must play a penalty card. Either one will do. 2♣ is illegal.

*Example (`no_hearts`).* Hearts are led, and 10♥ is the highest so far. You hold 9♥ and A♥. **Legal: 9♥ or A♥.** Every heart is a penalty card, so following suit already satisfies the rule.

### Rule N2: Forced penalty discard (card-based contracts only)

If you can't follow suit and you hold any penalty card, **you must discard a penalty card**. If you hold several, you choose which one.

*Example (`no_king_of_hearts`).* ♣ is led. You have no clubs. You hold K♥, 4♥ and 8♠. **Legal: K♥ only.**

*Example (`no_hearts`).* ♣ is led. You have no clubs. You hold K♥, 4♥ and 8♠. **Legal: K♥ or 4♥.**

In trick-based contracts, if you can't follow suit, you can discard any card.

### Rule N3: Heart-lead restriction (`no_hearts` and `no_king_of_hearts` only)

You can't lead a heart until a heart has been played on an earlier trick, **unless you hold only hearts**. (Since nobody can lead hearts before then, that first heart was always a discard.)

### Rule N4: Early end (card-based contracts only)

The hand ends **at the end of the trick in which the last penalty card is played**. For example, a `no_king_of_hearts` hand ends as soon as the trick containing K♥ is complete. Trick-based contracts are always played to 13 tricks.

## Game score

A bot's game score is the sum of its scores over the 20 hands. Over a full game, all scores add up to **0**: the negatives total −5200 (2 × each contract) and the trumps total +5200 (8 × 650).

## Match

- A match is 100 games: **25 deal sets × 4 seat rotations**. Within a deal set, all 20 deals are fixed per seat. In rotation `r`, bot `i` sits in seat `(i + r) mod 4`. So every bot plays every seat's cards against the same opponents.
- All deals come from one match seed, which is printed and saved. The same seed always gives the same deals.
- **Placement points per game:** 1st = 3, 2nd = 2, 3rd = 1, 4th = 0. Tied bots split the points of the positions they share (a tie for 1st gives each 2.5). **The bot with the most placement points wins the match.** Total points and games won are also reported.

## Faults

- **Timeout** (default 2s, set per match) or an **illegal answer**: the server plays your **lowest legal** option for you and records a **strike**.
  - For a card, lowest means the lowest rank, with ties broken by suit in the order ♣ ♦ ♥ ♠.
  - For a contract, it's the first legal option in this order: `no_tricks`, `no_hearts`, `no_queens`, `no_kings_jacks`, `no_last_two`, `no_king_of_hearts`, `trump` ♣/♦/♥/♠.
- **Disconnect:** the match pauses until your bot reconnects under the same name. A disconnect costs no strike, and the timer doesn't run while you're disconnected.

## Wire names

- Card: `{"rank": 12, "suit": "S"}`. Rank is 2–14 (J = 11, Q = 12, K = 13, A = 14). Suit is `C`, `D`, `H` or `S`.
- Contract: `{"kind": "no_queens"}` or `{"kind": "trump", "suit": "S"}`.
