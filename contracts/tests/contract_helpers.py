from __future__ import annotations

from typing import Any
from typing import cast
from typing import List
from typing import Sequence
from typing import Tuple

import attr
from brownie.network.state import Chain
from brownie.network.transaction import TransactionReceipt
from eth_typing import Address
from eth_typing import Hash32
from eth_utils import decode_hex
from eth_utils import keccak
from eth_utils import to_canonical_address
from py_ecc import bn128
from py_ecc.fields import bn128_FQ
from py_ecc.typing import Point2D

from tests.bls import BLSPublicKey

ZERO_ADDRESS = Address(b"\x00" * 20)
ZERO_HASH32 = Hash32(b"\x00" * 32)


@attr.s(auto_attribs=True, frozen=True)
class BatchConfig:
    start_batch_index: int
    start_block_number: int
    active: bool
    keypers: List[Address]
    threshold: int
    batch_span: int
    batch_size_limit: int
    transaction_size_limit: int
    transaction_gas_limit: int
    fee_receiver: Address
    target_address: Address
    target_function_selector: bytes
    execution_timeout: int

    @classmethod
    def from_tuple_without_keypers(
        cls, t: Tuple[Any, ...], keypers: Sequence[Address]
    ) -> BatchConfig:
        tuple_with_keypers = t[:3] + (list(keypers),) + t[3:]
        return cls.from_tuple(tuple_with_keypers)

    @classmethod
    def from_tuple(cls, t: Tuple[Any, ...]) -> BatchConfig:
        assert len(t) == 13
        return cls(
            start_batch_index=t[0],
            start_block_number=t[1],
            active=t[2],
            keypers=[to_canonical_address(keyper) for keyper in t[3]],
            threshold=t[4],
            batch_span=t[5],
            batch_size_limit=t[6],
            transaction_size_limit=t[7],
            transaction_gas_limit=t[8],
            fee_receiver=to_canonical_address(t[9]),
            target_address=to_canonical_address(t[10]),
            target_function_selector=decode_hex(str(t[11])),
            execution_timeout=t[12],
        )


ZERO_CONFIG = BatchConfig(
    start_batch_index=0,
    start_block_number=0,
    active=False,
    keypers=[],
    threshold=0,
    batch_span=0,
    batch_size_limit=0,
    transaction_size_limit=0,
    transaction_gas_limit=0,
    fee_receiver=ZERO_ADDRESS,
    target_address=ZERO_ADDRESS,
    target_function_selector=b"\x00\x00\x00\x00",
    execution_timeout=0,
)


def fetch_config_by_index(config_contract: Any, config_index: int) -> BatchConfig:
    config_tuple = config_contract.configs(config_index)
    config_num_keypers = config_contract.configNumKeypers(config_index)
    config_keypers = []
    for keyper_index in range(config_num_keypers):
        keyper = config_contract.configKeypers(config_index, keyper_index)
        config_keypers.append(to_canonical_address(keyper))

    return BatchConfig.from_tuple_without_keypers(config_tuple, config_keypers)


def fetch_next_config(config_contract: Any) -> BatchConfig:
    next_config_tuple = config_contract.nextConfig()
    next_config_num_keypers = config_contract.nextConfigNumKeypers()
    next_config_keypers = []
    for keyper_index in range(next_config_num_keypers):
        keyper = config_contract.nextConfigKeypers(keyper_index)
        next_config_keypers.append(to_canonical_address(keyper))

    return BatchConfig.from_tuple_without_keypers(next_config_tuple, next_config_keypers)


def fetch_config(config_contract: Any, batch_index: int) -> BatchConfig:
    full_config_tuple = config_contract.getConfig(batch_index)
    return BatchConfig.from_tuple(full_config_tuple)


def set_next_config(config_contract: Any, config: BatchConfig, owner: Address) -> None:
    for field in attr.fields(BatchConfig):
        name_snake_case = field.name
        name_camel_case = snake_to_camel_case(name_snake_case, capitalize=True)
        if name_snake_case == "keypers":
            continue

        setter_function_name = "nextConfigSet" + name_camel_case
        setter_function = getattr(config_contract, setter_function_name)
        value = getattr(config, name_snake_case)
        setter_function(value, {"from": owner})

    num_existing_keypers = config_contract.nextConfigNumKeypers()
    config_contract.nextConfigRemoveKeypers(num_existing_keypers, {"from": owner})
    config_contract.nextConfigAddKeypers(config.keypers, {"from": owner})


def schedule_config(
    config_contract: Any, config: BatchConfig, owner: Address
) -> TransactionReceipt:
    set_next_config(config_contract, config, owner=owner)
    tx = config_contract.scheduleNextConfig({"from": owner})
    return tx


def mine_until(block_number: int, chain: Chain) -> None:
    current_block_number = chain.height
    assert current_block_number <= block_number
    blocks_to_mine = block_number - current_block_number
    chain.mine(blocks_to_mine)


def snake_to_camel_case(snake_case_string: str, capitalize: bool) -> str:
    if len(snake_case_string) == 0:
        return ""
    parts = snake_case_string.split("_")

    if capitalize:
        first_part = parts[0].capitalize()
    else:
        first_part = parts[0]

    rest_parts = [part.capitalize() for part in parts[1:]]

    return "".join([first_part] + rest_parts)


def compute_batch_hash(batch: Sequence[bytes]) -> bytes:
    result = bytes(ZERO_HASH32)
    for tx in batch:
        result = keccak(tx + result)
    return result


def hash_to_g1(message: bytes) -> Point2D[bn128_FQ]:
    message_hash_bytes = keccak(message)
    message_hash_int = int.from_bytes(message_hash_bytes, "big")
    message_hash_point = bn128.multiply(bn128.G1, message_hash_int)
    return message_hash_point


def public_key_to_contract_format(
    public_key: BLSPublicKey,
) -> Tuple[Tuple[int, int], Tuple[int, int]]:
    """Convert a BLSPublicKey to the format expected by the contracts."""
    return (
        (cast(int, public_key[0].coeffs[1]), cast(int, public_key[0].coeffs[0])),
        (cast(int, public_key[1].coeffs[1]), cast(int, public_key[1].coeffs[0])),
    )


def compute_decrypted_transaction_hash(transactions: Sequence[bytes]) -> Hash32:
    decrypted_transaction_hash = bytes(ZERO_HASH32)
    for transaction in transactions:
        decrypted_transaction_hash = keccak(transaction + decrypted_transaction_hash)
    return Hash32(decrypted_transaction_hash)


def compute_decryption_signature_preimage(
    *,
    batcher_contract_address: Address,
    cipher_batch_hash: Hash32,
    decryption_key: bytes,
    decrypted_transactions: Sequence[bytes],
) -> bytes:
    decrypted_transaction_hash = compute_decrypted_transaction_hash(decrypted_transactions)
    preimage = b"".join(
        [batcher_contract_address, cipher_batch_hash, decryption_key, decrypted_transaction_hash]
    )
    return preimage
