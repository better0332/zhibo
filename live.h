#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>
#include <sys/stat.h>
#include <sys/statfs.h>
#include <stdio.h>
#include <sys/ipc.h>
#include <sys/msg.h>
#include <stdint.h>

#define SRCV_CMD 1
#define MSG_SIZE 8192

struct message
{
    long type;
    long pid;
    char cmd[MSG_SIZE];
};

int read_live_info_from_IPC(uint32_t *live_id,uint8_t *live_status,uint8_t *category)
{
	int msqid = msgget(ftok("/home/icache/zhibo",1), IPC_CREAT|0666);
	struct message msg;	
	
	msgrcv(msqid, &msg, MSG_SIZE+sizeof(msg.pid), SRCV_CMD, 0);
	*live_status = msg.cmd[0];
	*category = msg.cmd[1];
	memcpy(live_id,msg.cmd+2,4);
	
	sprintf(msg.cmd, "REV live_id=%u,status=%u\n",*live_id,*live_status);
	msg.type = msg.pid;
	msgsnd(msqid, &msg, strlen(msg.cmd)+1+sizeof(msg.pid), 0);
	return 1;
}

