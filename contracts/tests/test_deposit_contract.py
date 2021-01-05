from typing import Any

import brownie
from brownie.network.account import Account
from brownie.network.account import Accounts
from brownie.network.contract import ContractTx
from brownie.network.state import Chain
from brownie.network.web3 import Web3
from eth_utils import decode_hex

from tests.contract_helpers import mine_until


def check_deposit(
    deposit_contract: Any,
    account: Account,
    amount: int,
    withdrawal_delay_blocks: int,
    withdrawal_requested_block: int,
) -> None:
    assert deposit_contract.getDepositAmount(account) == amount
    assert deposit_contract.getWithdrawalDelayBlocks(account) == withdrawal_delay_blocks
    assert deposit_contract.getWithdrawalRequestedBlock(account) == withdrawal_requested_block


def check_deposit_event(
    tx: ContractTx, amount: int, withdrawal_delay_blocks: int, withdrawal_requested_block: int,
) -> None:
    assert "Deposited" in tx.events and len(tx.events["Deposited"]) == 1
    event = tx.events["Deposited"][0]
    assert event["account"] == tx.sender
    assert event["amount"] == amount
    assert event["withdrawalDelayBlocks"] == withdrawal_delay_blocks


def test_deposit(deposit_contract: Any, deposit_token_contract: Any, owner: Account) -> None:
    # data must be 8 bytes
    invalid_data = ["", "0x000000", "0x0000000000"]
    for data in invalid_data:
        with brownie.reverts():
            deposit_token_contract.send(deposit_contract, 100, data, {"from": owner})

    valid_data = "0x00000000aabbccdd"
    withdrawal_delay_blocks = int.from_bytes(decode_hex(valid_data), byteorder="big")

    tx = deposit_token_contract.send(deposit_contract, 100, valid_data, {"from": owner})
    check_deposit_event(tx, 100, withdrawal_delay_blocks, 0)
    check_deposit(deposit_contract, owner, 100, withdrawal_delay_blocks, 0)

    # depositing twice works, but only if withdrawal delay isn't decreased
    with brownie.reverts():
        deposit_token_contract.send(deposit_contract, 50, "0x00000000aabbccdc", {"from": owner})

    tx = deposit_token_contract.send(deposit_contract, 50, valid_data, {"from": owner})
    check_deposit_event(tx, 50, withdrawal_delay_blocks, 0)
    check_deposit(deposit_contract, owner, 150, withdrawal_delay_blocks, 0)

    tx = deposit_token_contract.send(deposit_contract, 0, "0x00000000aabbccde", {"from": owner})
    check_deposit_event(tx, 0, withdrawal_delay_blocks + 1, 0)
    check_deposit(deposit_contract, owner, 150, withdrawal_delay_blocks + 1, 0)


def test_withdraw(
    deposit_contract: Any,
    deposit_token_contract: Any,
    owner: Account,
    accounts: Accounts,
    chain: Chain,
    web3: Web3,
) -> None:
    withdrawal_delay = 10
    withdrawal_delay_data = withdrawal_delay.to_bytes(8, "big")
    deposit_token_contract.send(deposit_contract, 100, withdrawal_delay_data)
    recipient = accounts[-1]

    chain.mine(withdrawal_delay)
    with brownie.reverts():
        deposit_contract.withdraw(recipient, {"from": owner})

    tx = deposit_contract.requestWithdrawal({"from": owner})
    assert "WithdrawalRequested" in tx.events and len(tx.events["WithdrawalRequested"]) == 1
    event = tx.events["WithdrawalRequested"][0]
    assert event["account"] == owner
    assert deposit_contract.getWithdrawalRequestedBlock(owner) == tx.block_number
    withdraw_block_number = tx.block_number + withdrawal_delay

    with brownie.reverts():
        deposit_contract.withdraw(recipient, {"from": owner})
    mine_until(withdraw_block_number - 2, chain)
    with brownie.reverts():
        deposit_contract.withdraw(recipient, {"from": owner})
    assert web3.eth.blockNumber == withdraw_block_number - 1  # one block mined by sending tx
    tx = deposit_contract.withdraw(recipient, {"from": owner})

    assert "Withdrawn" in tx.events and len(tx.events["Withdrawn"]) == 1
    event = tx.events["Withdrawn"][0]
    assert event["account"] == owner
    assert event["recipient"] == recipient
    assert event["amount"] == 100

    check_deposit(deposit_contract, owner, 0, 0, 0)
    assert deposit_token_contract.balanceOf(recipient) == 100
