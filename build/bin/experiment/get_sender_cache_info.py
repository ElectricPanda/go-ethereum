cleanCacheHitTotalTime = 0
cleanCacheHitCount = 0
levelDbCacheHitTotalTime = 0
levelDbCacheHitCount = 0
levelDbCacheMissTotalTime = 0
levelDbCacheMissCount = 0

with open('file.txt') as f:
    for line in f:
        time_str = line.split('Took time: ')[1].split()[0]
        time = int(time_str)

        if 'CleanCacheHitData: true' in line:
            cleanCacheHitTotalTime += time
            cleanCacheHitCount += 1
        elif 'LevelDBCacheMiss: true' in line:
            levelDbCacheMissTotalTime += time
            levelDbCacheMissCount += 1
        else:
            levelDbCacheHitTotalTime += time
            levelDbCacheHitCount += 1

if cleanCacheHitCount == 0:
    print("No rows with CleanCacheHitData = true")
else:
    cleanCacheHitAvgTime = cleanCacheHitTotalTime / cleanCacheHitCount
    print(f"Total Retrieve Time: {cleanCacheHitTotalTime+levelDbCacheMissTotalTime+levelDbCacheHitTotalTime}")
    print(f"Total cleanCacheHitTotalTime: {cleanCacheHitTotalTime}")
    print(f"Total CleanCacheHit cleanCacheHitCount: {cleanCacheHitCount}")
    print(f"Average CleanCacheHit Retrieve time: {cleanCacheHitAvgTime}")
if levelDbCacheHitCount == 0:
    print("No rows with LevelDBCacheHit")
else:
    levelDbCacheHitAvgTime = levelDbCacheHitTotalTime / levelDbCacheHitCount
    print(f"Total Retrieve Time: {cleanCacheHitTotalTime+levelDbCacheMissTotalTime+levelDbCacheHitTotalTime}")
    print(f"Total levelDbCacheHitTotalTime: {levelDbCacheHitTotalTime}")
    print(f"Total LevelDBCacheHit levelDbCacheHitCount: {levelDbCacheHitCount}")
    print(f"Average LevelDBCacheHit Retrieve time: {levelDbCacheHitAvgTime}")

if levelDbCacheMissCount == 0:
    print("No rows with LevelDBCacheMiss")
else:
    levelDbCacheMissAvgTime = levelDbCacheMissTotalTime / levelDbCacheMissCount
    print(f"Total Retrieve Time: {cleanCacheHitTotalTime+levelDbCacheMissTotalTime+levelDbCacheHitTotalTime}")
    print(f"Total levelDbCacheMissTotalTime: {levelDbCacheMissTotalTime}")
    print(f"Total LevelDBCacheMiss levelDbCacheMissCount: {levelDbCacheMissCount}")
    print(f"Average LevelDBCacheMiss Retrieve time: {levelDbCacheMissAvgTime}")