from typing import Any
from typing import List

from brownie.network.account import Account
from brownie.network.state import Chain
from eth_typing import Hash32
from eth_utils import to_canonical_address

from tests import ecdsa
from tests.contract_helpers import compute_decryption_signature_preimage
from tests.contract_helpers import mine_until
from tests.contract_helpers import schedule_config
from tests.contract_helpers import ZERO_HASH32
from tests.factories import make_batch_config
from tests.factories import make_bytes


def test_accusing(
    keyper_slasher: Any,
    config_contract: Any,
    executor_contract: Any,
    chain: Chain,
    owner: Account,
    keypers: List[Account],
    config_change_heads_up_blocks: int,
) -> None:
    config = make_batch_config(
        start_batch_index=0,
        start_block_number=chain.height + config_change_heads_up_blocks + 20,
        batch_span=10,
        keypers=keypers,
        threshold=0,
    )
    schedule_config(config_contract, config, owner=owner)
    mine_until(config.start_block_number + config.batch_span * 10, chain)

    executor_contract.executeCipherBatch(ZERO_HASH32, [], 0, {"from": keypers[0]})
    tx = keyper_slasher.accuse(0, 1, {"from": keypers[1]})

    assert len(tx.events) == 1
    assert len(tx.events["Accused"]) == 1
    event = tx.events["Accused"][0]

    assert event["halfStep"] == 0
    assert event["executor"] == keypers[0]
    assert event["accuser"] == keypers[1]

    accusation = keyper_slasher.accusations(0)
    assert accusation[0]  # accused
    assert not accusation[1]  # appealed
    assert accusation[2] == keypers[0]  # executor
    assert accusation[3] == 0  # half step
    assert accusation[4] == tx.block_number


def test_appealing(
    keyper_slasher: Any,
    config_contract: Any,
    executor_contract: Any,
    mock_batcher_contract: Any,
    keypers: List[Account],
    keyper_private_keys: List[bytes],
    chain: Chain,
    owner: Account,
    config_change_heads_up_blocks: int,
) -> None:
    print(keypers[0])
    config = make_batch_config(
        start_batch_index=0,
        start_block_number=chain.height + config_change_heads_up_blocks + 20,
        batch_span=10,
        keypers=keypers,
        threshold=0,
    )
    schedule_config(config_contract, config, owner=owner)
    mine_until(config.start_block_number + config.batch_span * 10, chain)

    cipher_batch_hash = Hash32(make_bytes(32))
    mock_batcher_contract.setBatchHash(0, 0, cipher_batch_hash)
    decrypted_transactions = [make_bytes() for _ in range(3)]
    decryption_signature_preimage = compute_decryption_signature_preimage(
        batcher_contract_address=to_canonical_address(mock_batcher_contract.address),
        cipher_batch_hash=cipher_batch_hash,
        decrypted_transactions=decrypted_transactions,
    )
    print(decryption_signature_preimage)
    signatures = [ecdsa.sign(key, decryption_signature_preimage) for key in keyper_private_keys]

    executor_contract.executeCipherBatch(
        cipher_batch_hash, decrypted_transactions, 0, {"from": keypers[0]}
    )
    keyper_slasher.accuse(0, 1, {"from": keypers[1]})

    authorization = (
        0,
        ZERO_HASH32,
        list(range(len(keypers))),
        signatures,
    )
    tx = keyper_slasher.appeal(authorization)

    assert len(tx.events) == 1
    assert len(tx.events["Appealed"]) == 1
    event = tx.events["Appealed"]
    assert event["halfStep"] == 0
    assert event["executor"] == keypers[0]

    accusation = keyper_slasher.accusations(0)
    assert accusation[1]  # appealed


def test_slashing(
    keyper_slasher: Any,
    config_contract: Any,
    executor_contract: Any,
    chain: Chain,
    owner: Account,
    keypers: List[Account],
    config_change_heads_up_blocks: int,
    appeal_blocks: int,
) -> None:
    config = make_batch_config(
        start_batch_index=0,
        start_block_number=chain.height + config_change_heads_up_blocks + 20,
        batch_span=10,
        keypers=keypers,
        threshold=0,
    )
    schedule_config(config_contract, config, owner=owner)
    mine_until(config.start_block_number + config.batch_span * 10, chain)

    executor_contract.executeCipherBatch(ZERO_HASH32, [], 0, {"from": keypers[0]})
    tx = keyper_slasher.accuse(0, 1, {"from": keypers[1]})
    mine_until(tx.block_number + appeal_blocks, chain)
    tx = keyper_slasher.slash(0)

    assert len(tx.events) == 1
    assert len(tx.events["Slashed"]) == 1
    event = tx.events["Slashed"][0]
    assert event["halfStep"] == 0
    assert event["executor"] == keypers[0]
