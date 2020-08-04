import csv
import matplotlib as mpl
mpl.use('Agg')
import matplotlib.pyplot as plt
import numpy as np
import sys
from itertools import zip_longest
from pathlib import Path

IMPTLOGPATH ='imptData/impt_data_blockNum_indexed_block_process_time/impt_log_files/'
IMPTCSVPATH = IMPTLOGPATH + 'csv/'
IMPTGRAPHPATH = IMPTLOGPATH + 'graph/'
ORIGINALLOGPATH = 'imptData/impt_data_original_geth_block_process_time/impt_log_files/'
ORIGINALCSVPATH = ORIGINALLOGPATH + 'csv/'
ORIGINALGRAPHPATH = ORIGINALLOGPATH + 'graph/'

# make empty 2d list -> list[b][a]
def TwoD(a, b):
    return np.zeros(a * b, dtype=int).reshape(b, a)

# make the directory if not exist
def makeDir(path):
    Path(path).mkdir(parents=True, exist_ok=True) 



# analyze impt_block_process_time.txt log files
def analyzeBlockProcessTimeLog(isIMPT):

    # MAXBLOCKNUM = 500000
    MAXBLOCKNUM = 10
    TIMESNUM = 7
    graphNames = ["totalCommitTxTime", "commitTxCount", "bodyWriteTime", "headerWriteTime", "stateFlushTime", "dirtyNodesCount", "blockInsertedTime"]
    
    if isIMPT:
        LOGFILEPATH = IMPTLOGPATH + 'impt_block_process_time.txt'
        GRAPHPATH = IMPTGRAPHPATH + 'blockProcessTime/'
    else:
        LOGFILEPATH = ORIGINALLOGPATH + 'impt_block_process_time.txt'
        GRAPHPATH = ORIGINALGRAPHPATH + 'blockProcessTime/'
    makeDir(GRAPHPATH)

    # times[0][blockNum] = totalCommitTxTime (ns)
    # times[1][blockNum] = commitTxCount
    # times[2][blockNum] = bodyWriteTime (ns)
    # times[3][blockNum] = headerWriteTime (ns)
    # times[4][blockNum] = stateFlushTime (ns)
    # times[5][blockNum] = dirtyNodesCount
    # times[6][blockNum] = blockInsertedTime(Unix time with ns)
    times = TwoD(MAXBLOCKNUM+1, TIMESNUM)

    # read data from log file
    f = open(LOGFILEPATH, 'r')
    rdr = csv.reader(f)
    cnt = 0
    blockNum = 1
    for line in rdr:
        if len(line) == 0:
            continue

        line = line[0].split(":")[:-1]

        if line[0][0] == 'c':
            # print("commitTxTime ->", line[1], "ns")
            times[0][blockNum] = times[0][blockNum] + int(line[1])
            times[1][blockNum] = times[1][blockNum] + 1
        elif line[0][0] == 'b':
            # print("blockWriteTime (body, header)->", line[2], "ns /", line[3], "ns")
            times[2][blockNum] = int(line[2])
            times[3][blockNum] = int(line[3])
        elif line[0][0] == 's':
            # print("stateFlushTime ->", line[1], "ns")
            times[4][blockNum] = int(line[1])
        elif line[0][0] == 'd':
            # print("# of dirte nodes ->", line[1])
            times[5][blockNum] = int(line[1])
        elif line[0][0] == 'i':
            # print("insertedblock (blockNum, insertedTime) ->", "block", line[1], "/ at" , line[2], "nano unix time")
            times[6][blockNum] = int(line[2])
            print("at block", line[1], "avg commit time:", int(times[0][blockNum]/times[1][blockNum]))
            if blockNum == MAXBLOCKNUM:
                break
            blockNum = blockNum + 1
            
        else:
            print("unknown error")
            sys.exit()
    f.close()



    # draw graphs
    print("Drawing graphs...")
    blockNums = list(range(1,MAXBLOCKNUM+1))

    # draw average commit tx time
    times[1][0] = 1 # to avoid DIVBYZERO error
    avgCommitTxTime = [int(totalTime/txCount) for totalTime,txCount in zip(times[0], times[1])]



    # make log data as a csv file
    data = [blockNums, times[0][1:], times[1][1:], avgCommitTxTime[1:], times[2][1:], times[3][1:], times[4][1:], times[5][1:], times[6][1:]]
    export_data = zip_longest(*data, fillvalue = '')
    if isIMPT:
        csvFilePath = IMPTCSVPATH
    else:
        csvFilePath = ORIGINALCSVPATH
    with open(csvFilePath + "block_process_time.csv", 'w', encoding="ISO-8859-1", newline='') as myfile:
        wr = csv.writer(myfile)
        wr.writerow(("block number", "total commit tx time (ns)", "# of commit tx", "avg commit tx time (ns)", "body write time (ns)", "header write time (ns)", "state flush time (ns)", "# of dirty trie nodes", "block inserted unix time (ns)"))
        wr.writerows(export_data)
    myfile.close()



    maxAvgCommitTime = 20000000
    for i in range(len(avgCommitTxTime)):
        if avgCommitTxTime[i] > maxAvgCommitTime:
            avgCommitTxTime[i] = maxAvgCommitTime
    plt.figure()
    plt.title('averageCommitTxTime', pad=10)                # set graph title
    plt.xlabel('block num', labelpad=10)                    # set x axis
    plt.ylabel('average commit tx time (ns)', labelpad=10)  # set y axis
    plt.scatter(blockNums, avgCommitTxTime[1:], s=1)        # draw scatter graph
    graphName = 'averageCommitTxTime'
    plt.savefig(GRAPHPATH+graphName)

    # draw other graphs
    for i in range(2, TIMESNUM):
        plt.figure()
        ylabel = graphNames[i]
        if i != 5:
            ylabel = ylabel + '(ns)'
        plt.title(graphNames[i], pad=10)            # set graph title
        plt.xlabel('block num', labelpad=10)        # set x axis
        plt.ylabel(ylabel, labelpad=10)             # set y axis
        plt.scatter(blockNums, times[i][1:], s=1)   # draw scatter graph
        graphName = graphNames[i]
        plt.savefig(GRAPHPATH+graphName)



# analyze impt_database_inspect.txt log files
def analyzeDatabaseInspectLog(isIMPT):

    # db inspect epoch (per x blocks)
    DBINSPECTEPOCH = 10000

    # meaning of each log line (21 contents) (delimiter: ',') (unit: KB) (type: float)
    # headerSize, bodySize, receiptSize, tdSize, numHashPairing, hashNumPairing, txlookupSize, bloomBitsSize, trieSize, preimageSize,
    # cliqueSnapsSize, metadata, ancientHeaders, ancientBodies, ancientReceipts, ancientTds, ancientHashes, chtTrieNodes, bloomTrieNodes, total, unaccounted
    SIZESNUM = 21
    graphNames = ["headerSize", "bodySize", "receiptSize", "tdSize", "numHashPairing", "hashNumPairing", "txlookupSize", "bloomBitsSize", "trieSize", "preimageSize",
    "cliqueSnapsSize", "metadata", "ancientHeaders", "ancientBodies", "ancientReceipts", "ancientTds", "ancientHashes", "chtTrieNodes", "bloomTrieNodes", "total", "unaccounted"]

    if isIMPT:
        LOGFILEPATH = IMPTLOGPATH + 'impt_database_inspect.txt'
        GRAPHPATH = IMPTGRAPHPATH + 'databaseInspect/'
    else:
        LOGFILEPATH = ORIGINALLOGPATH + 'impt_database_inspect.txt'
        GRAPHPATH = ORIGINALGRAPHPATH + 'databaseInspect/'
    makeDir(GRAPHPATH)

    LINENUM = sum(1 for line in open(LOGFILEPATH))
    sizes = TwoD(LINENUM, SIZESNUM) # sizes[contents index] = list of its inspected sizes

    # read data from log file
    f = open(LOGFILEPATH, 'r')
    rdr = csv.reader(f)
    lineNum = 0
    for line in rdr:
        if len(line) == 0:
            continue

        for i in range(len(line)-1):
            sizes[i][lineNum] = float(line[i])

        lineNum = lineNum + 1

    f.close()

    # make log data as a csv file
    blockNums = list(range(1,LINENUM+1))
    blockNums = [DBINSPECTEPOCH*x for x in blockNums]
    data = [blockNums]
    for i in range(SIZESNUM):
        data.append(sizes[i])
    export_data = zip_longest(*data, fillvalue = '')
    if isIMPT:
        csvFilePath = IMPTCSVPATH
    else:
        csvFilePath = ORIGINALCSVPATH
    with open(csvFilePath + "database_inspect.csv", 'w', encoding="ISO-8859-1", newline='') as myfile:
        wr = csv.writer(myfile)
        wr.writerow(["all sizes are KB"])
        wr.writerow(("block number", "headerSize", "bodySize", "receiptSize", "tdSize", "numHashPairing", "hashNumPairing", "txlookupSize", "bloomBitsSize", "trieSize", "preimageSize",
    "cliqueSnapsSize", "metadata", "ancientHeaders", "ancientBodies", "ancientReceipts", "ancientTds", "ancientHashes", "chtTrieNodes", "bloomTrieNodes", "total", "unaccounted"))
        wr.writerows(export_data)
    myfile.close()

    # draw graphs
    print("Drawing graphs...")
    for i in range(SIZESNUM):
        plt.figure()
        plt.title(graphNames[i], pad=10)                    # set graph title
        plt.xlabel('block num', labelpad=10)                # set x axis
        plt.ylabel(graphNames[i] + ' (KB)', labelpad=10)    # set y axis
        plt.plot(blockNums, sizes[i])                       # draw scatter graph

        # save graph
        graphName = graphNames[i]
        plt.savefig(GRAPHPATH+graphName)



# analyze impt_data_log.txt log files
def analyzeDatabaseReadTimeLog(isIMPT):

    # num of leveldb for indexed trie nodes (1 means no additional db for trie nodes)
    TRIEDBNUM = 1

    MAXBLOCKNUM = 2200
    DATANUM = 3

    if isIMPT:
        LOGFILEPATH = IMPTLOGPATH + 'impt_data_log.txt'
        GRAPHPATH = IMPTGRAPHPATH + 'databaseReadTime/'
    else:
        LOGFILEPATH = ORIGINALLOGPATH + 'impt_data_log.txt'
        GRAPHPATH = ORIGINALGRAPHPATH + 'databaseReadTime/'
    makeDir(GRAPHPATH)

    # logDatas[0][blockNum] = total database read time (ns)
    # logDatas[1][blockNum] = # of database read
    # logDatas[2][blockNum] = database size (KB)
    logDatas = TwoD(MAXBLOCKNUM+1, DATANUM)

    # read data from log file
    f = open(LOGFILEPATH, 'r')
    rdr = csv.reader(f)
    blockNum = 1
    for line in rdr:
        if len(line) == 0:
            continue

        lastElement = line[-1]
        line = line[:-1]
        line = list(map(int, line))

        if len(line) != 0 and lastElement != '@':
            # get db search times
            logDatas[0][blockNum] = logDatas[0][blockNum] + int(line[2])
            logDatas[1][blockNum] = logDatas[1][blockNum] + 1
        else:
            # get db size
            logDatas[2][blockNum] = int(line[2])
            if blockNum == MAXBLOCKNUM:
                break
            blockNum = blockNum + 1

    f.close()



    # make log data as a csv file
    blockNums = list(range(1,MAXBLOCKNUM+1))
    data = [blockNums, logDatas[0][1:], logDatas[1][1:], logDatas[2][1:]]
    export_data = zip_longest(*data, fillvalue = '')
    if isIMPT:
        csvFilePath = IMPTCSVPATH
    else:
        csvFilePath = ORIGINALCSVPATH
    with open(csvFilePath + "database_read_time.csv", 'w', encoding="ISO-8859-1", newline='') as myfile:
        wr = csv.writer(myfile)
        wr.writerow(("block number", "total database read time (ns)", "# of database read", "database size (KB)"))
        wr.writerows(export_data)
    myfile.close()



if __name__ == "__main__":

    # make directory to save files/graphs
    makeDir(IMPTCSVPATH)
    makeDir(IMPTGRAPHPATH)
    makeDir(ORIGINALCSVPATH)
    makeDir(ORIGINALGRAPHPATH)

    # analyze impt_block_process_time.txt log files
    analyzeBlockProcessTimeLog(True)
    analyzeBlockProcessTimeLog(False)

    # analyze impt_database_inspect.txt log files
    analyzeDatabaseInspectLog(True)
    analyzeDatabaseInspectLog(False)

    # analyze impt_data_log.txt log files
    analyzeDatabaseReadTimeLog(True)
    analyzeDatabaseReadTimeLog(False)

    print("Done")


