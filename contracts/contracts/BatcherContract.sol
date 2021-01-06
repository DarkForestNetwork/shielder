// SPDX-License-Identifier: MIT

pragma solidity >=0.7.0 <0.8.0;
pragma experimental ABIEncoderV2;

import "OpenZeppelin/openzeppelin-contracts@3.3.0/contracts/access/Ownable.sol";
import "./ConfigContract.sol";
import "./FeeBankContract.sol";

enum TransactionType {Cipher, Plain}

/// @title A contract that batches transactions.
contract BatcherContract is Ownable {
    /// @notice The event emitted whenever a transaction is added to a batch.
    /// @param batchIndex The index of the batch to which the transaction has been added.
    /// @param transactionType The type of the transaction (cipher or plain).
    /// @param transaction The encrypted or plaintext transaction (depending on the type).
    /// @param batchHash The batch hash after adding the transaction.
    event TransactionAdded(
        uint64 batchIndex,
        TransactionType transactionType,
        bytes transaction,
        bytes32 batchHash
    );

    // The contract from which batch configs are fetched.
    ConfigContract public configContract;
    // The contract to which fees are sent.
    FeeBankContract public feeBankContract;

    // Stores the current size of the batches by batch index. Note that cipher and plain batches
    // are not tracked separately but in sum.
    mapping(uint64 => uint64) public batchSizes;
    // The current batch hashes by index and type (cipher or plain).
    mapping(uint64 => mapping(TransactionType => bytes32)) public batchHashes;

    // The minimum fee required to add a transaction to a batch.
    uint64 public minFee;

    constructor(address _configContractAddress, address _feeBankAddress) {
        configContract = ConfigContract(_configContractAddress);
        feeBankContract = FeeBankContract(_feeBankAddress);
    }

    /// @notice Add a transaction to a batch.
    /// @param _batchIndex The index of the batch to which the transaction should be added. Note
    ///     that this must match the batch corresponding to the current block number.
    /// @param _type The type of the transaction (either cipher or plain).
    /// @param _transaction The encrypted or plaintext transaction (depending on `_type`).
    function addTransaction(
        uint64 _batchIndex,
        TransactionType _type,
        bytes calldata _transaction
    ) external payable {
        BatchConfig memory config = configContract.getConfig(_batchIndex);

        // check batching is active
        require(config.batchSpan > 0);

        // check given batch is current
        assert(_batchIndex >= config.startBatchIndex); // ensured by configContract.getConfig
        uint64 _relativeBatchIndex = _batchIndex - config.startBatchIndex;
        uint64 _batchStartBlock = config.startBlockNumber +
            _relativeBatchIndex *
            config.batchSpan;
        uint64 _batchEndBlock = _batchStartBlock + config.batchSpan;
        require(block.number >= _batchStartBlock);
        require(block.number < _batchEndBlock);

        // check tx and batch size limits
        require(0 < _transaction.length);
        require(_transaction.length <= config.transactionSizeLimit);
        require(
            batchSizes[_batchIndex] + _transaction.length <=
                config.batchSizeLimit
        ); // overflow can be ignored here because number of txs and their sizes are both small

        // check fee
        require(msg.value >= minFee);

        // add tx to batch
        bytes memory _batchHashPreimage = abi.encodePacked(
            _transaction,
            batchHashes[_batchIndex][_type]
        );
        bytes32 _newBatchHash = keccak256(_batchHashPreimage);
        batchHashes[_batchIndex][_type] = _newBatchHash;
        batchSizes[_batchIndex] += uint64(_transaction.length);

        // pay fee to fee bank and emit event
        if (msg.value > 0 && config.feeReceiver != address(0)) {
            feeBankContract.deposit{value: msg.value}(config.feeReceiver);
        }
        emit TransactionAdded(_batchIndex, _type, _transaction, _newBatchHash);
    }

    /// @notice Set the minimum fee required to add a transaction to the batch.
    /// @param _minFee The new value for the minimum fee.
    function setMinFee(uint64 _minFee) external onlyOwner {
        minFee = _minFee;
    }
}
