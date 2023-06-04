

























#include<stdio.h>
int main()
{
int num,i=0;
printf("enter the value\n");
scanf("%i",&num);
int space =num-1;
for(int i=num*2;i>1;i=i+2)
{
for(int k=space;k>0;k=k-1){
printf(" ");

}
space=space-1;

for(int j=0;j<i;j=j+1)
{
printf("*");
}
printf("\n");
}
}





















