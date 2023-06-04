#include<stdio.h>
#define AB  10
int s = AB * 100;
#undef AB
int main()
{
printf(" the value of s=%d\n",s);
}
